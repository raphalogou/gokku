package dokku

import (
	"regexp"
	"slices"
)

// Identifier rules, checked before any argument reaches dokku (CMD-1). They follow
// Dokku's own and match the CHECK constraints of migrations/00002_dokku_name_rules.sql.
var (
	// Dokku 0.38: lowercase alphanumerics, dots and dashes, starting with a letter or
	// digit (plugins/common/common.go, IsValidAppName). Dokku sets no length limit.
	appNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,127}$`)
	// dokku-datastore 0.10: letters, digits, underscores and dashes. A leading dash is
	// refused because a command would read it as a flag.
	serviceNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,127}$`)
	// Plugin names as printed by plugin:list.
	pluginNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
)

// ValidAppName reports whether name is a valid Dokku app name.
func ValidAppName(name string) bool { return appNameRe.MatchString(name) }

// ValidServiceName reports whether name is a valid datastore service name.
func ValidServiceName(name string) bool { return serviceNameRe.MatchString(name) }

// Datastores are the service plugins Gokku supports, matched against plugin:list (SVC-1).
var Datastores = []string{
	"postgres", "mysql", "mariadb", "mongo", "redis", "rabbitmq",
	"memcached", "elasticsearch", "clickhouse", "couchdb",
}

// IsDatastore reports whether typ is a supported datastore plugin.
func IsDatastore(typ string) bool { return slices.Contains(Datastores, typ) }
