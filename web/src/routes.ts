import { index, layout, type RouteConfig, route } from "@react-router/dev/routes";

// Screens and routes follow DESIGN.md, section 7.
export default [
  layout("routes/public-layout.tsx", [
    route("login", "routes/login.tsx"),
    route("setup", "routes/setup.tsx"),
    route("invite/:token", "routes/invite.tsx"),
  ]),
  layout("routes/app-layout.tsx", [
    index("routes/index.tsx"),
    route("profile", "routes/profile.tsx"),
    route("users", "routes/users.tsx"),
    route("projects", "routes/projects.tsx", [route("new", "routes/projects-new.tsx")]),
    route("projects/:slug", "routes/project.tsx", [
      index("routes/project-overview.tsx"),
      route("apps", "routes/project-apps.tsx", [
        route("new", "routes/apps-new.tsx", { id: "project-apps-new" }),
      ]),
      route("services", "routes/project-services.tsx", [
        route("new", "routes/services-new.tsx", { id: "project-services-new" }),
      ]),
      route("config", "routes/project-config.tsx"),
      route("activity", "routes/project-activity.tsx"),
      route("settings", "routes/project-settings.tsx"),
    ]),
    route("apps", "routes/apps.tsx", [route("new", "routes/apps-new.tsx")]),
    route("apps/:name", "routes/app.tsx", [
      index("routes/app-overview.tsx"),
      route("config", "routes/app-config.tsx"),
      route("domains", "routes/app-domains.tsx"),
      route("logs", "routes/app-logs.tsx"),
      route("scaling", "routes/app-scaling.tsx"),
      route("deploy", "routes/app-deploy.tsx"),
      route("settings", "routes/app-settings.tsx"),
    ]),
    route("apps/:name/deploys/:jobId", "routes/deploy-output.tsx"),
    route("jobs/:id", "routes/job.tsx"),
    route("services", "routes/services.tsx", [route("new", "routes/services-new.tsx")]),
    route("services/:type/:name", "routes/service.tsx"),
    route("plugins", "routes/plugins.tsx"),
    route("activity", "routes/activity.tsx"),
    route("settings", "routes/settings.tsx"),
    route("*", "routes/not-found.tsx"),
  ]),
] satisfies RouteConfig;
