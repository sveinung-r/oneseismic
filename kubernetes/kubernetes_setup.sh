#!/usr/bin/env bash

source .env

az aks create \
    -o table \
    -c $NODE_COUNT \
    -g $RESOURCE_GROUP \
    -n $CLUSTER_NAME \
    --vnet-subnet-id ${SUBNET} \
    --network-plugin azure \
    --service-cidr ${SERVICE_CIDR} \
    --dns-service-ip ${DNS_IP} \
    --generate-ssh-keys \
    --enable-managed-identity

az aks get-credentials --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME

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
    -o table \
    -n $IP_NAME \
    --resource-group $NODE_RESOURCE_GROUP \
    --allocation-method Static \
    --sku Standard

IP=$(az network public-ip list --query "[?name=='$IP_NAME'].ipAddress" -o tsv)

# Update public ip address with DNS name
az network public-ip update \
    -o table \
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
  AZURE_STORAGE_ACCOUNT: $(echo -n $AZURE_STORAGE_ACCOUNT | base64 -w 0)
  AZURE_MANIFEST_CONTAINER: $(echo -n $AZURE_MANIFEST_CONTAINER | base64 -w 0)
  AUTHSERVER: $(echo -n $AUTHSERVER | base64 -w 0)
  ISSUER: $(echo -n $ISSUER | base64 -w 0)
  AZURE_STORAGE_URL: $(echo -n $AZURE_STORAGE_URL | base64 -w 0)
  LOG_LEVEL: $(echo -n $LOG_LEVEL | base64 -w 0)
  CLIENT_SECRET: $(echo -n $CLIENT_SECRET | base64 -w 0)
  AUDIENCE: $(echo -n $AUDIENCE | base64 -w 0)
  CLIENT_ID: $(echo -n $CLIENT_ID | base64 -w 0)
" | kubectl apply -f -

# Create a namespace for your ingress resources
#kubectl create namespace ingress-basic

# Add the official stable repo
helm repo add stable https://kubernetes-charts.storage.googleapis.com/

# Use Helm to deploy an NGINX ingress controller
helm install nginx stable/nginx-ingress \
    --set controller.replicaCount=1 \
    --set controller.config.large-client-header-buffers="4 64k" \
    --set controller.service.loadBalancerIP=${IP} \
    --set controller.nodeSelector."beta\.kubernetes\.io/os"=linux \
    --set defaultBackend.nodeSelector."beta\.kubernetes\.io/os"=linux

# Install the CustomResourceDefinition resources separately
kubectl apply --validate=false -f https://raw.githubusercontent.com/jetstack/cert-manager/release-0.13/deploy/manifests/00-crds.yaml

# Label the ingress-basic namespace to disable resource validation
#kubectl label namespace ingress-basic cert-manager.io/disable-validation=true

# Add the Jetstack Helm repository
helm repo add jetstack https://charts.jetstack.io

# Update your local Helm chart repository cache
helm repo update

# Install the cert-manager Helm chart
helm install \
  cert-manager \
  --version v0.13.0 \
  jetstack/cert-manager

DOMAIN_NAME=$(az network public-ip list --query "[?name=='$IP_NAME'].dnsSettings.fqdn" -o tsv)

sleep 1m

cat kubernetes/oneseismic.yml | \
    sed s/'PUBLIC_IP'/${IP}/ | \
    sed s/'DOMAIN_NAME'/${DOMAIN_NAME}/ | \
    sed s/'EMAIL'/${EMAIL}/ | \
    kubectl apply --validate=false -f -

cat kubernetes/filebeat-kubernetes.yaml | \
    sed s/elasticsearch_host/$ELASTICSEARCH_HOST/ | \
    sed s/changeme/$ELASTICSEARCH_PASSWORD/ | \
    kubectl apply --validate=false -f -

cat kubernetes/metricbeat-kubernetes.yaml | \
    sed s/elasticsearch_host/$ELASTICSEARCH_HOST/ | \
    sed s/changeme/$ELASTICSEARCH_PASSWORD/ | \
    kubectl apply --validate=false -f -

echo "Your onesesmic cluster is up and running! Listening on: $IP"
echo "Domain name: $DOMAIN_NAME"
