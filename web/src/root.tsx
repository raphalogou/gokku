import "@fontsource/hanken-grotesk/400.css";
import "@fontsource/hanken-grotesk/500.css";
import "@fontsource/hanken-grotesk/600.css";
import "@fontsource/hanken-grotesk/700.css";
import "@fontsource/jetbrains-mono/400.css";
import "@fontsource/jetbrains-mono/500.css";
import "./styles/app.css";

import { CSPProvider } from "@base-ui/react/csp-provider";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Links, Meta, Outlet, Scripts, ScrollRestoration } from "react-router";
import { ToastProvider } from "./components/ui/toast";
import { queryClient } from "./lib/query-client";
import { strings } from "./lib/strings";

// Sets data-theme before first paint from the cached preference (DESIGN.md, 5.3).
// The server allows this script by its hash in the Content-Security-Policy, so any change
// here is picked up automatically on the next build.
const themeScript = `(function(){var t="system";try{t=localStorage.getItem("gokku-theme")||t}catch(e){}if(t==="system")t=matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light";document.documentElement.dataset.theme=t})()`;

export function Layout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>{strings.product}</title>
        <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
        {/* biome-ignore lint/security/noDangerouslySetInnerHtml: constant script, allowed by CSP hash */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <Meta />
        <Links />
      </head>
      <body className="bg-bg text-ink antialiased">
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  return (
    // The CSP has no 'unsafe-inline' styles, so Base UI must not inject <style> elements;
    // app.css carries the rules it would have added.
    <CSPProvider disableStyleElements>
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <Outlet />
        </ToastProvider>
      </QueryClientProvider>
    </CSPProvider>
  );
}

export function HydrateFallback() {
  return null;
}
