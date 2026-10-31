package dokku

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// fakeSSL is an app's Let's Encrypt state in the fake runner.
type fakeSSL struct {
	active  bool
	expires time.Time
	email   string
}

// sslCommand answers domains:add, domains:remove and the letsencrypt commands. The caller
// holds f.mu.
func (f *FakeRunner) sslCommand(args []string) (script, bool) {
	if len(args) == 3 && (args[0] == "domains:add" || args[0] == "domains:remove") {
		a, ok := f.apps[args[1]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[1])), true
		}
		d := args[2]
		if args[0] == "domains:add" {
			if slices.Contains(a.domains, d) {
				return script{lines: out(fmt.Sprintf(" !     %s is already defined for %s", d, a.name))}, true
			}
			a.domains = append(a.domains, d)
			return script{lines: out("-----> Added "+d+" to "+a.name, "-----> Configuring "+d+"...", "-----> Reloading nginx")}, true
		}
		a.domains = slices.DeleteFunc(a.domains, func(x string) bool { return x == d })
		return script{lines: out("-----> Removed "+d+" from "+a.name, "-----> Reloading nginx")}, true
	}
	if !strings.HasPrefix(args[0], "letsencrypt:") {
		return script{}, false
	}
	if !f.installed("letsencrypt") {
		return failScript(args[0] + " is not a dokku command."), true
	}
	if args[0] == "letsencrypt:cron-job" && len(args) == 2 {
		f.autoRenew = args[1] == "--add"
		if f.autoRenew {
			return script{lines: out("-----> Added cron job to dokku's crontab.")}, true
		}
		return script{lines: out("-----> Removed cron job from dokku's crontab.")}, true
	}
	if len(args) < 2 {
		return script{}, false
	}
	a, ok := f.apps[args[1]]
	if !ok {
		return failScript(fmt.Sprintf("App %s does not exist", args[1])), true
	}
	switch {
	case args[0] == "letsencrypt:report" && len(args) == 2:
		return script{lines: out(f.letsencryptReport(a)...)}, true
	case args[0] == "letsencrypt:enable":
		if len(a.domains) == 0 {
			return failScript("No domains configured for " + a.name), true
		}
		if a.ssl.email == "" && f.globalEmail == "" {
			return failScript("Missing email address for app " + a.name + ", set one with letsencrypt:set"), true
		}
		a.ssl.active = true
		// Like Let's Encrypt: 90 days from a notBefore backdated by one hour.
		a.ssl.expires = time.Now().Add(90*24*time.Hour - time.Hour).Truncate(time.Second)
		lines := []string{"=====> Let's Encrypt " + a.name, "-----> Updating letsencrypt docker image..."}
		for _, d := range a.domains {
			lines = append(lines, "       - Domain '"+d+"'")
		}
		lines = append(lines, "-----> Getting letsencrypt certificate for "+a.name+" via HTTP-01",
			"       [INFO] [acme] Server responded with a certificate.",
			"-----> Certificate retrieved successfully.",
			"-----> Installing let's encrypt certificates", "-----> Reloading nginx")
		return script{lines: out(lines...)}, true
	case args[0] == "letsencrypt:disable" && len(args) == 2:
		a.ssl = fakeSSL{email: a.ssl.email}
		return script{lines: out("-----> Disabling letsencrypt for app", "-----> Removing certificates", "-----> Done")}, true
	case args[0] == "letsencrypt:set" && len(args) >= 3 && args[2] == "email":
		a.ssl.email = ""
		if len(args) == 4 {
			a.ssl.email = args[3]
		}
		return script{lines: out("=====> Setting email to " + a.ssl.email)}, true
	}
	return script{}, false
}

// letsencryptReport prints the app's section as the plugin's cmd-letsencrypt-report-single
// does: printf "%-30s %-25s" per key, keys capitalized.
func (f *FakeRunner) letsencryptReport(a *fakeApp) []string {
	exp := ""
	if !a.ssl.expires.IsZero() {
		exp = strconv.FormatInt(a.ssl.expires.Unix(), 10)
	}
	email := a.ssl.email
	if email == "" {
		email = f.globalEmail
	}
	lines := []string{"=====> " + a.name + " letsencrypt information"}
	for _, kv := range [][2]string{
		{"Letsencrypt active:", strconv.FormatBool(a.ssl.active)},
		{"Letsencrypt autorenew:", strconv.FormatBool(f.autoRenew)},
		{"Letsencrypt computed email:", email},
		{"Letsencrypt global email:", f.globalEmail},
		{"Letsencrypt email:", a.ssl.email},
		{"Letsencrypt expiration:", exp},
		{"Letsencrypt computed server:", "https://acme-v02.api.letsencrypt.org/directory"},
	} {
		lines = append(lines, "       "+fmt.Sprintf("%-30s %-25s", kv[0], kv[1]))
	}
	return lines
}
