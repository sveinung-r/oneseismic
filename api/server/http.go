package server

import (
	"fmt"
	"net/http"
	pprof "net/http/pprof"

	jwt "github.com/dgrijalva/jwt-go"
	_ "github.com/equinor/oneseismic/api/docs" // docs is generated by Swag CLI, you have to import it.
	l "github.com/equinor/oneseismic/api/logger"
	claimsmiddleware "github.com/equinor/oneseismic/api/middleware/claims"
	jwtmiddleware "github.com/iris-contrib/middleware/jwt"
	prometheusmiddleware "github.com/iris-contrib/middleware/prometheus"
	"github.com/iris-contrib/swagger/v12"
	"github.com/iris-contrib/swagger/v12/swaggerFiles"
	"github.com/kataras/iris/v12"
	irisCtx "github.com/kataras/iris/v12/context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPServer struct {
	manifestStore manifestStore
	app           *iris.Application
	hostAddr      string
	profile       bool
}

type HTTPServerOption interface {
	apply(*HTTPServer) error
}

func Create(c Config) (*HTTPServer, error) {
	app := iris.Default()
	app.Logger().SetPrefix("iris: ")
	l.AddGoLogSource(app.Logger().SetOutput)
	sURL, err := NewServiceURL(c.AzureBlobSettings)
	if err != nil {
		return nil, fmt.Errorf("creating ServiceURL: %w", err)
	}

	hs := HTTPServer{
		manifestStore: sURL,
		app:           app,
		hostAddr:      c.HostAddr}

	return &hs, nil
}

func Configure(hs *HTTPServer, opts ...HTTPServerOption) error {
	for _, opt := range opts {
		err := opt.apply(hs)
		if err != nil {
			return fmt.Errorf("Applying config failed: %v", err)
		}
	}

	hs.app.Use(iris.Gzip)
	hs.registerEndpoints()

	return nil
}

func WithOAuth2(oauthOpt OAuth2Option) HTTPServerOption {

	return newFuncOption(func(hs *HTTPServer) error {
		sigKeySet, err := GetOIDCKeySet(oauthOpt.AuthServer)
		if err != nil {
			return fmt.Errorf("Couldn't get keyset: %v", err)
		}

		rsaJWTHandler := jwtmiddleware.New(jwtmiddleware.Config{
			ValidationKeyGetter: func(t *jwt.Token) (interface{}, error) {

				if t.Method.Alg() != "RS256" {
					return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
				}
				return sigKeySet[t.Header["kid"].(string)], nil

			},
			ContextKey:    "user-jwt",
			SigningMethod: jwt.SigningMethodRS256,
		})

		onRS256Pass := func(ctx irisCtx.Context, err error) {

			if err == nil || err.Error() == "unexpected jwt signing method=RS256" {
				return
			}
			jwtmiddleware.OnError(ctx, err)
		}
		hmacJWTHandler := jwtmiddleware.New(jwtmiddleware.Config{
			ValidationKeyGetter: func(t *jwt.Token) (interface{}, error) {

				if t.Method.Alg() != "HS256" {
					return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
				}
				return oauthOpt.ApiSecret, nil
			},
			ContextKey:    "service-jwt",
			SigningMethod: jwt.SigningMethodHS256,
			ErrorHandler:  onRS256Pass,
		})

		if len(oauthOpt.Issuer) == 0 {
			oauthOpt.Issuer = oauthOpt.AuthServer.String()
		}

		claimsHandler := claimsmiddleware.New(oauthOpt.Audience, oauthOpt.Issuer)

		auth := func(ctx irisCtx.Context) {
			hmacJWTHandler.Serve(ctx)
			serviceToken := ctx.Values().Get("service-jwt")
			if serviceToken == nil {
				rsaJWTHandler.Serve(ctx)
			}

		}
		hs.app.Use(auth)
		hs.app.Use(claimsHandler.Validate)
		return nil
	})
}

func (hs *HTTPServer) registerEndpoints() {
	mc := manifestController{ms: hs.manifestStore}

	hs.app.Get("/", mc.list)
}

func (hs *HTTPServer) Serve() error {
	config := &swagger.Config{
		URL: fmt.Sprintf("http://%s/swagger/doc.json", hs.hostAddr), //The url pointing to API definition
	}
	// use swagger middleware to
	hs.app.Get("/swagger/{any:path}", swagger.CustomWrapHandler(config, swaggerFiles.Handler))

	if hs.profile {
		// Activate Prometheus middleware if profiling is on
		metrics := iris.Default()

		l.AddGoLogSource(metrics.Logger().SetOutput)
		metrics.Get("/metrics", iris.FromStd(promhttp.Handler()))
		metrics.Get("/debug/pprof", iris.FromStd(pprof.Index))
		metrics.Get("/debug/pprof/cmdline", iris.FromStd(pprof.Cmdline))
		metrics.Get("/debug/pprof/profile", iris.FromStd(pprof.Profile))
		metrics.Get("/debug/pprof/symbol", iris.FromStd(pprof.Symbol))

		metrics.Get("/debug/pprof/goroutine", iris.FromStd(pprof.Handler("goroutine")))
		metrics.Get("/debug/pprof/heap", iris.FromStd(pprof.Handler("heap")))
		metrics.Get("/debug/pprof/threadcreate", iris.FromStd(pprof.Handler("threadcreate")))
		metrics.Get("/debug/pprof/block", iris.FromStd(pprof.Handler("block")))

		err := metrics.Build()
		if err != nil {
			panic(err)
		}
		metricsServer := &http.Server{Addr: ":8081", Handler: metrics}

		go func() {
			err := metricsServer.ListenAndServe()
			if err != nil {
				l.LogE("Server shutdown", err)
			}
		}()
	}

	return hs.app.Run(iris.Addr(hs.hostAddr))
}

func WithProfiling() HTTPServerOption {

	return newFuncOption(func(hs *HTTPServer) (err error) {
		hs.profile = true

		m := prometheusmiddleware.New("Metrics", 0.3, 1.2, 5.0)
		hs.app.Use(m.ServeHTTP)
		hs.app.OnAnyErrorCode(func(ctx iris.Context) {
			// error code handlers are not sharing the same middleware as other routes, so we have
			// to call them inside their body.
			m.ServeHTTP(ctx)

		})
		return
	})
}
