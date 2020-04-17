package caddy2webdav

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/caddyhttp/httpserver"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/hacdias/webdav/v3/lib"
	"golang.org/x/net/webdav"
)

func init() {
	caddy.RegisterModule(webDav{})
	httpcaddyfile.RegisterHandlerDirective("webdav", parseCaddyfile)
}

type webDav struct {
	Configs []*config `json:"configs"`
}

type config struct {
	User    *user            `json:"user,nonempty"`
	Users   map[string]*user `json:"users,nonempty"`
	BaseURL string           `json:"base_url"`

	libWebdav *lib.Config
}

type user struct {
	Username string  `json:"username"`
	Password string  `json:"password"`
	Scope    string  `json:"scope"`
	Modify   bool    `json:"modify"`
	Rules    []*rule `json:"rules,nonempty"`
}

type rule struct {
	Regex  bool   `json:"regex"`
	Allow  bool   `json:"allow"`
	Path   string `json:"path"`
	Regexp string `json:"regexp"`
}

// CaddyModule returns the Caddy module information.
func (webDav) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.webdav",
		New: func() caddy.Module { return new(webDav) },
	}
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (wd *webDav) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	for i := range wd.Configs {
		// Checks if the current request is for the current configuration.
		if !httpserver.Path(r.URL.Path).Matches(wd.Configs[i].BaseURL) {
			continue
		}

		wd.Configs[i].libWebdav.ServeHTTP(w, r)
		return nil
	}

	return next.ServeHTTP(w, r)
}

// Provision implements caddy.Provisioner.
func (wd *webDav) Provision(ctx caddy.Context) error {
	for _, c := range wd.Configs {
		c.libWebdav = &lib.Config{
			Auth:  false, // Must use basicauth directive for this.
			Users: map[string]*lib.User{},
			User: &lib.User{
				Scope:  ".",
				Rules:  []*lib.Rule{},
				Modify: true,
			},
		}

		transform := func(cu *lib.User, u *user) {
			cu.Username = u.Username
			cu.Password = u.Password
			cu.Scope = u.Scope
			cu.Modify = u.Modify
			for _, r := range u.Rules {
				cr := &lib.Rule{
					Regex: r.Regex,
					Allow: r.Allow,
					Path:  r.Path,
				}
				if r.Regexp == "dotfiles" {
					cr.Regexp = regexp.MustCompile(`\/\..+`)
				} else {
					cr.Regexp = regexp.MustCompile(r.Regexp)
				}
				cu.Rules = append(cu.Rules, cr)
			}

			cu.Handler = &webdav.Handler{
				Prefix:     c.BaseURL,
				FileSystem: webdav.Dir(u.Scope),
				LockSystem: webdav.NewMemLS(),
			}
		}

		transform(c.libWebdav.User, c.User)
		for username, u := range c.Users {
			cu := &lib.User{}
			transform(cu, u)
			c.libWebdav.Users[username] = cu
		}

	}

	return nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (wd *webDav) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		conf := &config{
			User: &user{
				Scope:  ".",
				Rules:  []*rule{},
				Modify: true,
			},
			Users: map[string]*user{},
		}

		args := d.RemainingArgs()

		if len(args) > 0 {
			conf.BaseURL = args[0]
		}

		if len(args) > 1 {
			return d.ArgErr()
		}

		conf.BaseURL = strings.TrimSuffix(conf.BaseURL, "/")
		conf.BaseURL = strings.TrimPrefix(conf.BaseURL, "/")
		conf.BaseURL = "/" + conf.BaseURL

		if conf.BaseURL == "/" {
			conf.BaseURL = ""
		}

		u := conf.User

		for d.NextBlock(0) {
			switch d.Val() {
			case "scope":
				if !d.NextArg() {
					return d.ArgErr()
				}

				u.Scope = d.Val()
			case "allow", "allow_r", "block", "block_r":
				ruleType := d.Val()

				if !d.NextArg() {
					return d.ArgErr()
				}

				if d.Val() == "dotfiles" && !strings.HasSuffix(ruleType, "_r") {
					ruleType += "_r"
				}

				rule := &rule{
					Allow: ruleType == "allow" || ruleType == "allow_r",
					Regex: ruleType == "allow_r" || ruleType == "block_r",
				}

				if rule.Regex {
					if d.Val() == "dotfiles" {
						rule.Regexp = "dotfiles"
					} else {
						rule.Regexp = d.Val()
						_, err := regexp.Compile(rule.Regexp)
						if err != nil {
							return err
						}
					}
				} else {
					rule.Path = d.Val()
				}

				u.Rules = append(u.Rules, rule)
			case "modify":
				if !d.NextArg() {
					u.Modify = true
					continue
				}

				val, err := strconv.ParseBool(d.Val())
				if err != nil {
					return err
				}

				u.Modify = val
			default:
				if d.NextArg() {
					return d.ArgErr()
				}

				val := d.Val()
				if !strings.HasSuffix(val, ":") {
					return d.ArgErr()
				}

				val = strings.TrimSuffix(val, ":")

				conf.Users[val] = &user{
					Rules:  conf.User.Rules,
					Scope:  conf.User.Scope,
					Modify: conf.User.Modify,
				}

				u = conf.Users[val]
			}
		}

		wd.Configs = append(wd.Configs, conf)
	}

	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	d := &webDav{}
	err := d.UnmarshalCaddyfile(h.Dispenser)
	return d, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*webDav)(nil)
	_ caddyhttp.MiddlewareHandler = (*webDav)(nil)
	_ caddyfile.Unmarshaler       = (*webDav)(nil)
)
