#!/usr/bin/env bash

source .env

EMAIL=ssru@equinor.com
NODE_COUNT=5
RESOURCE_GROUP="OneSeismic"
CLUSTER_NAME="OneCluserPlayground"

az aks create \
    -c $NODE_COUNT \
    -g $RESOURCE_GROUP \
    -n $CLUSTER_NAME \
    --generate-ssh-keys \
    --enable-managed-identity

az aks get-credentials --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME

echo "Aks cluster created and credentials loaded"

ACR_NAME=oneseismic.azurecr.io
ACR_UNAME=$(az acr credential show -n $ACR_NAME --query="username" -o tsv)
ACR_PASSWD=$(az acr credential show -n $ACR_NAME --query="passwords[0].value" -o tsv)

kubectl create secret docker-registry acr-secret \
    --docker-server=$ACR_NAME \
    --docker-username=$ACR_UNAME \
    --docker-password=$ACR_PASSWD

# Create a public ip
NODE_RESOURCE_GROUP=$(az aks list --query "[?name=='$CLUSTER_NAME'].nodeResourceGroup" -o tsv)
IP_NAME="${CLUSTER_NAME}_public_ip"
az network public-ip create \
    -n $IP_NAME \
    --resource-group $NODE_RESOURCE_GROUP \
    --allocation-method Static \
    --sku Standard

IP=$(az network public-ip list --query "[?name=='$IP_NAME'].ipAddress" -o tsv)

# Update public ip address with DNS name
az network public-ip update \
    --name $IP_NAME \
    --dns-name $(echo $CLUSTER_NAME | tr '[:upper:]' '[:lower:]') \
    --resource-group $NODE_RESOURCE_GROUP

echo \
"
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
type: Opaque
data:
  AZURE_STORAGE_ACCESS_KEY: $(echo -n $AZURE_STORAGE_ACCESS_KEY | base64 -w 0)
  AZURE_STORAGE_ACCOUNT: $(echo -n $AZURE_STORAGE_ACCOUNT | base64 -w 0)
  AZURE_MANIFEST_CONTAINER: $(echo -n $AZURE_MANIFEST_CONTAINER | base64 -w 0)
  AUTHSERVER: $(echo -n $AUTHSERVER | base64 -w 0)
  ISSUER: $(echo -n $ISSUER | base64 -w 0)
  AZURE_STORAGE_URL: $(echo -n $AZURE_STORAGE_URL | base64 -w 0)
  API_SECRET: $(echo -n $API_SECRET | base64 -w 0)
  LOG_LEVEL: $(echo -n $LOG_LEVEL | base64 -w 0)
" | kubectl apply -f -

# Create a namespace for your ingress resources
#kubectl create namespace ingress-basic

# Add the official stable repo
helm repo add stable https://kubernetes-charts.storage.googleapis.com/

# Use Helm to deploy an NGINX ingress controller
helm install nginx stable/nginx-ingress \
    --set controller.replicaCount=1 \
    --set controller.service.loadBalancerIP=${IP} \
    --set controller.nodeSelector."beta\.kubernetes\.io/os"=linux \
    --set defaultBackend.nodeSelector."beta\.kubernetes\.io/os"=linux

# Install the CustomResourceDefinition resources separately
#kubectl apply --validate=false -f https://raw.githubusercontent.com/jetstack/cert-manager/release-0.13/deploy/manifests/00-crds.yaml

# Label the ingress-basic namespace to disable resource validation
#kubectl label namespace ingress-basic cert-manager.io/disable-validation=true

# Add the Jetstack Helm repository
#helm repo add jetstack https://charts.jetstack.io

# Update your local Helm chart repository cache
#helm repo update

# Install the cert-manager Helm chart
#helm install \
#  cert-manager \
#  --namespace ingress-basic \
#  --version v0.13.0 \
#  jetstack/cert-manager
#
#echo \
#"
#apiVersion: cert-manager.io/v1alpha2
#kind: ClusterIssuer
#metadata:
#  name: letsencrypt
#spec:
#  acme:
#    server: https://acme-v02.api.letsencrypt.org/directory
#    email: $EMAIL
#    privateKeySecretRef:
#      name: letsencrypt
#    solvers:
#    - http01:
#      ingress:
#      class: nginx
#" | kubectl apply -f -

DOMAIN_NAME=$(az network public-ip list --query "[?name=='$IP_NAME'].dnsSettings.fqdn" -o tsv)

openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -out aks-ingress-tls.crt \
    -keyout aks-ingress-tls.key \
    -subj "/CN=$DOMAIN_NAME/O=aks-ingress-tls"

kubectl create secret tls aks-ingress-tls \
    --key aks-ingress-tls.key \
    --cert aks-ingress-tls.crt

cat oneseismic.yml | sed s/'PUBLIC_IP'/${IP}/ | sed s/'DOMAIN_NAME'/${DOMAIN_NAME}/ | kubectl apply -f -

echo "Your onesesmic cluster is up and running! Listening on: $IP"
echo "Domain name: $DOMAIN_NAME"
