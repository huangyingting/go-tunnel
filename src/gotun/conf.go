// conf.go -- config file processing.
//
// Author: Sudhi Herle <sudhi@herle.net>
//
// This software does not come with any express or implied
// warranty; it is provided "as is". No claim  is made to its
// suitability for any purpose.

package main

import (
	"io"
	"fmt"
	"strings"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
)

// List of config entries
type Conf struct {
	Logging  string `yaml:"log"`
	LogLevel string `yaml:"loglevel"`
	Listen   []ListenConf `yaml:"listen"`
}

type ListenConf struct {
	Addr   string   `yaml:"address"`
	Allow  []subnet `yaml:"allow"`
	Deny   []subnet `yaml:"deny"`

	// optional TLS info; will listen on TLS socket if provided
	Tls    *TlsServerConf  `yaml:"tls"`

	// rate limit -- perhost and global
	Ratelimit *RateLimit `yaml:"ratelimit"`

	Connect  ConnectConf `yaml:"connect"`

}

type RateLimit struct {
	Global  int `yaml:"global"`
	PerHost int `yaml:"perhost"`
}

// An IP/Subnet
type subnet struct {
	net.IPNet
}

// Connect info
type ConnectConf struct {
	Addr    string `yaml:"address"`
	Bind    string
	Tls     *TlsClientConf `yaml:"tls"`
}

// Tls Conf
type TlsServerConf struct {
	Sni     bool
	Certdir string
	Cert    string
	Key     string
	ClientAuth string
	ClientCA string
}

// Tls client conf
type TlsClientConf struct {
	Cert    string
	Ca      string
	Key     string

	Server	string	`yaml:"servername"`
}


// Custom unmarshaler for IPNet
func (ipn *subnet) UnmarshalYAML(unm func(v interface{}) error) error {
	var s string

	// First unpack the bytes as a string. We then parse the string
	// as a CIDR
	err := unm(&s)
	if err != nil {
		return err
	}

	_, net, err := net.ParseCIDR(s)
	if err == nil {
		ipn.IP = net.IP
		ipn.Mask = net.Mask
	}
	return err
}


// Parse config file in YAML format and return
func ReadYAML(fn string) (*Conf, error) {
	yml, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("Can't read config file %s: %s", fn, err)
	}

	var cfg Conf
	err = yaml.Unmarshal(yml, &cfg)
	if err != nil {
		return nil, fmt.Errorf("Can't parse config file %s: %s", fn, err)
	}

	if err = validate(&cfg); err != nil {
		return nil, err
	}
	return defaults(&cfg), nil
}


// Setup sane defaults if needed
func defaults(c *Conf) *Conf {
	for _, l := range c.Listen {
		if l.Ratelimit == nil {
			l.Ratelimit = &RateLimit{}
		}
		if l.Ratelimit.Global == 0 {
			l.Ratelimit.Global = 1000
		}
		if l.Ratelimit.PerHost == 0 {
			l.Ratelimit.PerHost = 10
		}

	}

	if len(c.LogLevel) == 0 {
		c.LogLevel = "INFO"
	}

	if len(c.Logging) == 0 {
		c.Logging = "SYSLOG"
	}

	return c
}

// basic sanity check on the parsed config file
func validate(c *Conf) error {
	for _, l := range c.Listen {
		c := &l.Connect
		if len(c.Addr) == 0 {
			return fmt.Errorf("listener %s has missing connect info", l.Addr)
		}

		i := strings.IndexByte(l.Addr, ':')
		if i < 0 {
			return fmt.Errorf("%s: listen address is missing port", l.Addr)
		}

		if i = strings.IndexByte(c.Addr, ':'); i < 0 {
			return fmt.Errorf("%s: Connect address %s is missing port", l.Addr, c.Addr)
		}
		nm := c.Addr[:i]

		if t := c.Tls; t != nil {
			if len(t.Ca) == 0 {
				return fmt.Errorf("%s: TLS connect requires a valid CA", l.Addr)
			}
			if ip := net.ParseIP(nm); ip == nil {
				if len(t.Server) == 0 {
					t.Server = nm
				}
			}
		}

		if t := l.Tls; t != nil {
			if t.Sni {
				if len(t.Certdir) == 0 {
					return fmt.Errorf("%s: TLS SNI requires a certificate dir", l.Addr)
				}
			} else {
				if len(t.Cert) == 0 || len(t.Key) == 0 {
					return fmt.Errorf("%s: TLS server requires a valid certificate & key", l.Addr)
				}
			}

			auth := strings.ToLower(t.ClientAuth)
			switch auth {
			case "required", "optional":
				if len(t.ClientCA) == 0 {
					return fmt.Errorf("%s: TLS client-auth requires a valid CA certificate", l.Addr)
				}

			case "no", "disabled", "false":
				break

			default:
				return fmt.Errorf("%s: unknown client-auth type %s", l.Addr, t.ClientAuth)
			}

			t.ClientAuth = auth
		}
	}
	return nil
}


// Print config in human readable format
func (c *Conf) Dump(w io.Writer) {
	fmt.Fprintf(w, "config: %d listeners\n", len(c.Listen))

	for _, l := range c.Listen {
		fmt.Fprintf(w, "listen on %s", l.Addr)
		if t := l.Tls; t != nil {
			if t.Sni {
				fmt.Fprintf(w, " with tls sni using certstore %s", t.Certdir)
			} else {
				fmt.Fprintf(w, " with tls using cert %s, key %s",
				t.Cert, t.Key)
			}
			if t.ClientAuth == "required" {
				fmt.Fprintf(w, " requiring client auth")
			} else if t.ClientAuth == "optional" {
				fmt.Fprintf(w, " with optional client auth")
			}
		}
		c := &l.Connect
		fmt.Fprintf(w, "\n\tconnect to %s", c.Addr)
		if len(c.Bind) > 0 {
			fmt.Fprintf(w, " from %s", c.Bind)
		}
		if t := c.Tls; t != nil {
			fmt.Fprintf(w, " using tls cert %s, key %s, ca %s",
				t.Cert, t.Key, t.Ca)
		}
		fmt.Fprintf(w, "\n")
	}
}