package routes

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gin-gonic/gin"
	"github.com/wrfly/testing-kit/util/tokenbucket"

	"github.com/wrfly/yasuser/config"
	stner "github.com/wrfly/yasuser/shortener"
	"github.com/wrfly/yasuser/types"
)

type server struct {
	domain string
	gaID   string
	limit  int64

	stener        stner.Shortener
	indexTemplate *template.Template
	fileMap       map[string]bool

	host string
	tb   map[string]*tokenbucket.Bucket
}

func newServer(conf config.SrvConfig, shortener stner.Shortener) server {
	srv := server{
		domain: conf.Domain,
		stener: shortener,
		gaID:   conf.GAID,
		limit:  conf.Limit,
	}
	srv.init()

	return srv
}

func (s *server) init() {
	fileMap := make(map[string]bool, len(AssetNames()))
	for _, fileName := range AssetNames() {
		fileMap[fileName] = true
	}
	s.fileMap = fileMap

	bs, err := Asset("index.html")
	if err != nil {
		panic(err)
	}
	s.indexTemplate, err = template.New("index").Parse(string(bs))
	if err != nil {
		panic(err)
	}

	u, err := url.Parse(s.domain)
	if err != nil {
		panic(err)
	}
	s.host = u.Host

	s.tb = make(map[string]*tokenbucket.Bucket, 0)
}

func (s *server) handleIndex() gin.HandlerFunc {
	curlUA := regexp.MustCompile("curl*")

	return func(c *gin.Context) {
		if matched := curlUA.MatchString(c.Request.UserAgent()); matched {
			// query from curl
			c.String(200, fmt.Sprintf("curl %s -d \"%s\"",
				s.domain, "http://longlonglong.com/long/long/long?a=1&b=2"))
		} else {
			// visit from a web browser
			s.indexTemplate.Execute(c.Writer, map[string]string{
				"domain": s.domain,
				"gaID":   s.gaID,
			})
		}
	}
}

func (s *server) handleURI() gin.HandlerFunc {

	return func(c *gin.Context) {
		URI := c.Param("URI")

		switch {
		case URI == "":
			c.String(404, fmt.Sprintln("not found"))

		case s.fileMap[URI]:
			// handle static files
			http.FileServer(&assetfs.AssetFS{
				Asset:     Asset,
				AssetDir:  AssetDir,
				AssetInfo: AssetInfo,
				Prefix:    "/",
			}).ServeHTTP(c.Writer, c.Request)

		default:
			// handle shortURL
			if shortURL, err := s.stener.Restore(URI); err != nil {
				c.String(http.StatusNotFound, fmt.Sprintln(err.Error()))
			} else {
				if shortURL.Expire != nil {
					c.Redirect(http.StatusTemporaryRedirect, shortURL.Ori)
				} else {
					c.Redirect(http.StatusPermanentRedirect, shortURL.Ori)
				}
			}
		}

	}
}

func (s *server) handleLongURL() gin.HandlerFunc {
	return func(c *gin.Context) {
		// rate limit
		IP := c.ClientIP()
		if tb, ok := s.tb[IP]; !ok {
			s.tb[IP] = tokenbucket.New(s.limit, time.Second)
		} else {
			if !tb.TakeOne() {
				c.String(http.StatusBadRequest, "rate exceeded\n")
				return
			}
		}

		buf := urlBufferPool.Get().([]byte)
		defer urlBufferPool.Put(buf)
		n, err := c.Request.Body.Read(buf)
		if err != io.EOF && err != nil {
			badRequest(c, err)
			return
		}
		if n > MAX_URL_LENGTH {
			badRequest(c, types.ErrURLTooLong)
			return
		}

		longURL := fmt.Sprintf("%s", buf[:n])
		if err := s.invalidURL(longURL); err != nil {
			badRequest(c, err)
			return
		}

		shortURL, err := s.stener.Shorten(longURL, generateOptions(c.Request.Header))
		if err != nil {
			badRequest(c, err)
			return
		}

		c.String(200, fmt.Sprintf("%s/%s\n", s.domain, shortURL.Short))
	}
}

func badRequest(c *gin.Context, err error) {
	c.String(http.StatusBadRequest, err.Error())
}

func generateOptions(h http.Header) *types.ShortOptions {
	var duration time.Duration = -1
	customURL := h.Get("CUSTOM")
	passWord := h.Get("PASS")
	ttl := h.Get("TTL")
	if ttl != "" {
		duration, _ = time.ParseDuration(ttl)
	}
	return &types.ShortOptions{
		Custom: customURL,
		TTL:    duration,
		Passwd: passWord,
	}
}

func (s *server) invalidURL(URL string) error {
	u, err := url.Parse(URL)
	if err != nil {
		return err
	}

	if u.Host == s.host {
		return types.ErrSameHost
	}

	switch u.Scheme {
	case "http", "https", "ftp", "tcp":
		return nil
	default:
		return types.ErrScheme
	}
}
