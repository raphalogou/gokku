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
      route("apps", "routes/project-apps.tsx"),
      route("services", "routes/project-services.tsx"),
      // Tabs of later milestones.
      route("config", "routes/later-tab.tsx", { id: "project-config" }),
      route("activity", "routes/project-activity.tsx"),
      route("settings", "routes/later-tab.tsx", { id: "project-settings" }),
    ]),
    route("apps", "routes/apps.tsx"),
    route("apps/:name", "routes/app.tsx", [
      index("routes/app-overview.tsx"),
      route("config", "routes/later-tab.tsx", { id: "app-config" }),
      route("domains", "routes/later-tab.tsx", { id: "app-domains" }),
      route("logs", "routes/app-logs.tsx"),
      route("scaling", "routes/later-tab.tsx", { id: "app-scaling" }),
      route("deploy", "routes/later-tab.tsx", { id: "app-deploy" }),
      route("settings", "routes/later-tab.tsx", { id: "app-settings" }),
    ]),
    route("jobs/:id", "routes/job.tsx"),
    route("services", "routes/services.tsx"),
    route("plugins", "routes/plugins.tsx"),
    // Screens of later milestones, so navigation works and says what is coming.
    route("activity", "routes/activity.tsx"),
    route("settings", "routes/later.tsx", { id: "settings" }),
    route("*", "routes/not-found.tsx"),
  ]),
] satisfies RouteConfig;
