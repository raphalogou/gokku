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
    // Screens of later milestones, so navigation works and says what is coming.
    route("projects", "routes/later.tsx", { id: "projects" }),
    route("apps", "routes/later.tsx", { id: "apps" }),
    route("services", "routes/later.tsx", { id: "services" }),
    route("plugins", "routes/later.tsx", { id: "plugins" }),
    route("activity", "routes/later.tsx", { id: "activity" }),
    route("settings", "routes/later.tsx", { id: "settings" }),
    route("*", "routes/not-found.tsx"),
  ]),
] satisfies RouteConfig;
