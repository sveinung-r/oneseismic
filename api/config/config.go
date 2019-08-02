package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	authServer *url.URL
}

var cfg *Config

func SetDefaults() {
	viper.SetDefault("NO_AUTH", false)
	viper.SetDefault("AUTHSERVER", "http://oauth2.example.com")
	viper.SetDefault("MANIFEST_PATH", "tmp/")
	viper.SetDefault("HOST_ADDR", "http://localhost:8080")
	viper.SetDefault("DOMAIN_LIST", "")
	viper.SetDefault("DOMAIN_MAIL", "")
	viper.SetDefault("CERT_FILE", "cert.crt")
	viper.SetDefault("KEY_FILE", "cert.key")
	viper.SetDefault("HTTP_ONLY", false)
	viper.SetDefault("TLS", false)
	viper.SetDefault("LETSENCRYPT", false)
	viper.SetDefault("PROFILING", false)
	viper.SetDefault("SWAGGER", false)
}

func Load() error {
	cfg = new(Config)

	if !viper.GetBool("NO_AUTH") {
		a, err := parseURL(viper.GetString("AUTHSERVER"))
		if err != nil {
			return err
		}
		cfg.authServer = a
	}

	return nil
}

func parseURL(s string) (*url.URL, error) {

	if len(s) == 0 {
		return nil, fmt.Errorf("Url value empty")
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func AuthServer() *url.URL {

	return cfg.authServer
}

func UseAuth() bool {

	return !viper.GetBool("NO_AUTH")
}

func HostAddr() string {
	return viper.GetString("HOST_ADDR")
}

func StitchCmd() []string {
	return strings.Split(viper.GetString("STITCH_CMD"), " ")
}

func StitchAddr() string {
	return viper.GetString("STITCH_ADDR")
}

func ManifestStoragePath() string {
	return viper.GetString("MANIFEST_PATH")
}

func HttpOnly() bool {
	return viper.GetBool("HTTP_ONLY")
}

func UseTLS() bool {
	return viper.GetBool("TLS")
}

func UseLetsEncrypt() bool {
	return viper.GetBool("LETSENCRYPT")
}

func DomainList() string {
	return viper.GetString("DOMAIN_LIST")
}

func DomainMail() string {
	return viper.GetString("DOMAIN_MAIL")
}

func CertFile() string {
	return viper.GetString("CERT_FILE")
}

func KeyFile() string {
	return viper.GetString("KEY_FILE")
}
func ResourceID() string {
	return viper.GetString("RESOURCE_ID")
}
func Issuer() string {
	return viper.GetString("ISSUER")
}

func Profiling() bool {
	return viper.GetBool("PROFILING")
}

func Swagger() bool {
	return viper.GetBool("SWAGGER")
}
