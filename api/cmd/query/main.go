package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/equinor/oneseismic/api/api"
	"github.com/equinor/oneseismic/api/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/namsral/flag"
	"github.com/pebbe/zmq4"
)

type opts struct {
	authserver   string
	audience     string
	clientID     string
	clientSecret string
	storageURL   string
	redisURL     string
	bind         string
	signkey      string
}

func parseopts() (opts, error) {
	type option struct {
		param *string
		flag  string
		help  string
	}

	opts := opts {}
	params := []option {
		option {
			param: &opts.authserver,
			flag: "authserver",
			help: "OpenID Connect discovery server",
		},
		option {
			param: &opts.audience,
			flag: "audience",
			help: "Audience",
		},
		option {
			param: &opts.clientID,
			flag: "client-id",
			help: "Client ID",
		},
		option {
			param: &opts.clientSecret,
			flag: "client-secret",
			help: "Client Secret",
		},
		option {
			param: &opts.storageURL,
			flag: "storage-url",
			help: "Storage URL",
		},
		option {
			param: &opts.redisURL,
			flag: "redis-url",
			help: "Redis URL",
		},
		option {
			param: &opts.bind,
			flag: "bind",
			help: "Bind URL e.g. tcp://*:port",
		},
		option {
			param: &opts.signkey,
			flag:  "sign-key",
			help:  "Signing key used for response authorization tokens",
		},
	}

	for _, opt := range params {
		flag.StringVar(opt.param, opt.flag, "", opt.help)
	}
	flag.Parse()
	for _, opt := range params {
		if *opt.param == "" {
			return opts, fmt.Errorf("%s not set", opt.flag)
		}
	}

	return opts, nil
}

/*
 * Configuration for this instance of oneseismic for user-controlled clients
 *
 * Oneseismic does not really have a good concept of logged in users, sessions
 * etc. Rather, oneseismic gets tokens (in the Authorization header) which it
 * uses to obtain on-behalf-of tokens that in turn are used to query blob
 * storage. Users can use the python libraries to "log in", i.e. obtain a token
 * for their AD-registered user, constructed in a way that gives oneseismic the
 * permission to perform (blob) requests on their behalf [1].
 *
 * In order to construct a token that allows oneseismic to make requests, the
 * app-id of oneseismic must be available somehow. This app-id, sometimes
 * called client-id, is public information and for web apps often coded into
 * the javascript and ultimately delivered from server-side. Conceptually, this
 * should be no different. Oneseismic is largely designed to support multiple
 * deployments, so hard-coding an app id is probably not a good idea. Forcing
 * users to store or memorize the app-id and auth-server for use with the
 * python3 oneiseismic.login module is also not a good solution.
 *
 * The microsoft authentication library (MSAL) [2] is pretty clear on wanting a
 * client-id for obtaining a token. When the oneseismic python library is used,
 * it is an extension of the instance it's trying to reach, so getting the
 * app-id and authorization server [3] from a specific setup seems pretty
 * reasonable.
 *
 * The clientconfig struct and the /config endpoint are meant for sharing
 * oneseismic instance and company specific configurations with clients. While
 * only auth stuff is included now, it's a natural place to add more client
 * configuration parameters later e.g. performance hints, max/min latency.
 *
 * [1] https://docs.microsoft.com/en-us/graph/auth-v2-user
 * [2] https://msal-python.readthedocs.io/en/latest/#msal.PublicClientApplication
 * [3] usually https://login.microsoftonline.com/<tenant-id>
 *
 * https://docs.microsoft.com/en-us/azure/storage/common/storage-auth-aad-app
 */
type clientconfig struct {
	appid      string
	authority  string
	scopes     []string
}

func (c *clientconfig) Get(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H {
		/*
		 * oneseismic's app-id
		 */
		"client_id": c.appid,
		/*
		 * URL for the token authority. Usually
		 * https://login.microsoftonline.com/<tenant>
		 */
		"authority": c.authority,
		/*
		 * The scopes (permissions) that oneseismic requests in order to
		 * function
		 */
		"scopes": c.scopes,
	})
}

func main() {
	opts, err := parseopts()
	if err != nil {
		log.Fatalf("Unable to start server: %v", err)
	}

	httpclient := http.Client {
		Timeout: 10 * time.Second,
	}
	openidcfg, err := auth.GetOpenIDConfig(
		&httpclient,
		opts.authserver + "/v2.0/.well-known/openid-configuration",
	)
	if err != nil {
		log.Fatalf("Unable to get OpenID keyset: %v", err)
	}

	out, err := zmq4.NewSocket(zmq4.PUSH)
	if err != nil {
		log.Fatalf("Unable to create socket: %v", err)
	}
	err = out.Bind(opts.bind)
	if err != nil {
		log.Fatalf("Unable to bind queue to %s: %v", opts.bind, err)
	}
	defer out.Close()

	keyring := auth.MakeKeyring([]byte(opts.signkey))
	slice := api.MakeSlice(&keyring, opts.storageURL, out)
	result := api.Result {
		Timeout: time.Second * 15,
		StorageURL: opts.storageURL,
		Storage: redis.NewClient(&redis.Options {
			Addr: opts.redisURL,
			DB: 0,
		}),
		Keyring: &keyring,
	}

	cfg := clientconfig {
		appid: opts.clientID,
		authority: opts.authserver,
		scopes: []string{
			fmt.Sprintf("api://%s/One.Read", opts.clientID),
		},
	}

	validate := auth.ValidateJWT(openidcfg.Jwks, openidcfg.Issuer, opts.audience)
	onbehalf := auth.OnBehalfOf(openidcfg.TokenEndpoint, opts.clientID, opts.clientSecret)
	app := gin.Default()
	app.GET(
		"/query/:guid/slice/:dimension/:lineno",
		validate,
		onbehalf,
		slice.Get,
	)
	app.GET("/result/:pid", auth.ResultAuth(&keyring), result.Get)
	app.GET("/config", cfg.Get)
	app.Run(":8080")
}
