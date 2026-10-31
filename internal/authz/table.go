package authz

// Action is one permission. Every API route declares exactly one (PERM-1).
type Action string

// Actions from SPEC.md section 3.
const (
	MeView   Action = "me.view"
	MeUpdate Action = "me.update"

	AppView           Action = "app.view"
	AppLogsRead       Action = "app.logs.read"
	ProjectView       Action = "project.view"
	ServiceView       Action = "service.view"
	PluginView        Action = "plugin.view"
	JobView           Action = "job.view"
	AuditView         Action = "audit.view"
	AppSecretsReveal  Action = "app.secrets.reveal" //nolint:gosec // an action name, not a credential
	AppOperate        Action = "app.operate"
	AppConfigWrite    Action = "app.config.write"
	AppDomainsWrite   Action = "app.domains.write"
	AppSSLWrite       Action = "app.ssl.write"
	AppScale          Action = "app.scale"
	AppDeploy         Action = "app.deploy"
	ServiceLink       Action = "service.link"
	ServiceOperate    Action = "service.operate"
	ServiceSecretsRev Action = "service.secrets.reveal"
	ProjectOperate    Action = "project.operate"
	JobCancel         Action = "job.cancel"

	AppCreate           Action = "app.create"
	AppDestroy          Action = "app.destroy"
	AppRename           Action = "app.rename"
	AppMove             Action = "app.move"
	ServiceCreate       Action = "service.create"
	ServiceDestroy      Action = "service.destroy"
	ProjectCreate       Action = "project.create"
	ProjectUpdate       Action = "project.update"
	ProjectDelete       Action = "project.delete"
	ProjectMembersWrite Action = "project.members.write"
	ProjectEnvWrite     Action = "project.env.write"
	UserManage          Action = "user.manage"
	TokenManage         Action = "token.manage"
	SettingsWrite       Action = "settings.write"
	ServerRescan        Action = "server.rescan"
	AuditExport         Action = "audit.export"
)

// minRole is the minimum role per action, mirroring SPEC.md section 3. "Any signed-in user"
// is viewer, the lowest role. It is read-only after initialization.
var minRole = map[Action]Role{
	MeView:   RoleViewer,
	MeUpdate: RoleViewer,

	AppView:     RoleViewer,
	AppLogsRead: RoleViewer,
	ProjectView: RoleViewer,
	ServiceView: RoleViewer,
	PluginView:  RoleViewer,
	JobView:     RoleViewer,
	AuditView:   RoleViewer,

	AppSecretsReveal:  RoleOperator,
	AppOperate:        RoleOperator,
	AppConfigWrite:    RoleOperator,
	AppDomainsWrite:   RoleOperator,
	AppSSLWrite:       RoleOperator,
	AppScale:          RoleOperator,
	AppDeploy:         RoleOperator,
	ServiceLink:       RoleOperator,
	ServiceOperate:    RoleOperator,
	ServiceSecretsRev: RoleOperator,
	ProjectOperate:    RoleOperator,
	JobCancel:         RoleOperator,

	AppCreate:           RoleAdmin,
	AppDestroy:          RoleAdmin,
	AppRename:           RoleAdmin,
	AppMove:             RoleAdmin,
	ServiceCreate:       RoleAdmin,
	ServiceDestroy:      RoleAdmin,
	ProjectCreate:       RoleAdmin,
	ProjectUpdate:       RoleAdmin,
	ProjectDelete:       RoleAdmin,
	ProjectMembersWrite: RoleAdmin,
	ProjectEnvWrite:     RoleAdmin,
	UserManage:          RoleAdmin,
	TokenManage:         RoleAdmin,
	SettingsWrite:       RoleAdmin,
	ServerRescan:        RoleAdmin,
	AuditExport:         RoleAdmin,
}

// Known reports whether a is in the table.
func Known(a Action) bool {
	_, ok := minRole[a]
	return ok
}
