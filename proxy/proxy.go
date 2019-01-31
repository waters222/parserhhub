package proxy

import "C"
import (
	"context"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/weishi258/parserhhub/log"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type ProxyServer struct {
	localAddr      string
	server         *http.Server
	router         *mux.Router
	allowedHeaders []string
	allowedOrigins []string
	allowedMethods []string
}

func getDefaultAllowedHeaders() []string {
	return []string{"*"}
}
func getDefaultAllowedOrigins() []string {
	return []string{"*"}
}
func getDefaultAllowedMethods() []string {
	return []string{"GET", "POST"}
}
func NewProxyServer(localAddr string) *ProxyServer {
	ret := &ProxyServer{localAddr: localAddr,
		allowedHeaders: getDefaultAllowedHeaders(),
		allowedOrigins: getDefaultAllowedOrigins(),
		allowedMethods: getDefaultAllowedMethods()}
	ret.router = mux.NewRouter().SkipClean(true)

	return ret
}

func (c *ProxyServer) Start(sigChan chan bool, keepalive bool) error {
	if c.server != nil {
		return errors.New("rest server already started")
	}
	headersOk := handlers.AllowedHeaders(c.allowedHeaders)
	originsOk := handlers.AllowedOrigins(c.allowedOrigins)
	methodsOk := handlers.AllowedMethods(c.allowedMethods)
	for _, site := range c.allowedOrigins {
		log.GetLogger().Info("CORS allowed origins", zap.String("site", site))
	}
	for _, header := range c.allowedHeaders {
		log.GetLogger().Info("CORS allowed headers", zap.String("header", header))
	}

	for _, method := range c.allowedMethods {
		log.GetLogger().Info("CORS allowed methods", zap.String("method", method))
	}

	c.router.Methods("GET").HandlerFunc(GetHandler)
	c.router.Methods("POST").HandlerFunc(PostHandler)

	c.server = &http.Server{Addr: c.localAddr, Handler: handlers.CORS(headersOk, originsOk, methodsOk)(c.router)}
	c.server.SetKeepAlivesEnabled(keepalive)

	go func() {
		log.GetLogger().Info("proxy server started", zap.String("addr", c.localAddr))
		if err := c.server.ListenAndServe(); err != nil {
			log.GetLogger().Info("proxy Server stopped", zap.String("addr", c.localAddr), zap.String("cause", err.Error()))
			if sigChan != nil {
				sigChan <- true
			}
		}
	}()
	return nil
}

func (c *ProxyServer) Shutdown() error {
	if c.server == nil {
		return errors.New("proxy server not started")
	}
	log.GetLogger().Info("proxy Server is shutting down", zap.String("addr", c.localAddr))
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	if err := c.server.Shutdown(ctx); err != nil {
		log.GetLogger().Error("proxy Server shutdown failed", zap.String("error", err.Error()))
	} else {
		log.GetLogger().Info("proxy Server shutdown successful", zap.String("addr", c.localAddr))
	}
	c.server = nil
	return nil
}

func extractDest(path string) (string, error) {
	if strings.Index(path, "/proxy/") != 0 {
		return "", errors.New("unknown path")
	}
	return strings.Replace(path, "/proxy/", "", 1), nil
}

func doRequest(w http.ResponseWriter, method string, dest string, headers http.Header, requestBody io.Reader) {
	logger := log.GetLogger()

	req, err := http.NewRequest(method, dest, requestBody)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		logger.Error("create new get request failed", zap.String("error", err.Error()))
		return
	}
	req.Header = headers
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Error("request failed", zap.String("error", err.Error()))
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		logger.Error("write response body failed", zap.String("error", err.Error()))
	}
}

func GetHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLogger()

	headers := make([]string, 0)
	for k, v := range r.Header {
		headers = append(headers, fmt.Sprintf("%s:%s, ", k, v))
	}

	logger.Debug("GET proxy",
		zap.String("path", r.RequestURI),
		zap.String("user-agent", r.UserAgent()),
		zap.String("user-agent", r.UserAgent()), zap.Strings("headers", headers))

	dest, err := extractDest(r.RequestURI)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Debug("extract destination failed", zap.String("error", err.Error()))
		return
	}
	if len(dest) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	doRequest(w, "GET", dest, r.Header, nil)
}

func PostHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLogger()

	headers := make([]string, 0)
	for k, v := range r.Header {
		headers = append(headers, fmt.Sprintf("%s:%s, ", k, v))
	}

	if err := r.ParseForm(); err != nil {
		logger.Error("parse form failed", zap.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	forms := make([]string, 0)
	for k, v := range r.PostForm {
		if v != nil {
			forms = append(forms, fmt.Sprintf("%s:%s, ", k, v))
		}
	}

	logger.Debug("POST proxy",
		zap.String("path", r.RequestURI),
		zap.String("user-agent", r.UserAgent()),
		zap.Strings("headers", headers),
		zap.Strings("forms", forms))

	dest, err := extractDest(r.RequestURI)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		logger.Debug("extract destination failed", zap.String("error", err.Error()))
		return
	}
	if len(dest) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	doRequest(w, "POST", dest, r.Header, strings.NewReader(r.Form.Encode()))
}
