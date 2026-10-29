import type { ReactNode } from "react";
import { Links, Meta, Outlet, Scripts, ScrollRestoration } from "react-router";
import "./styles/app.css";

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
        <title>Gokku</title>
        <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
        {/* biome-ignore lint/security/noDangerouslySetInnerHtml: constant script, allowed by CSP hash */}
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <Meta />
        <Links />
      </head>
      <body>
        {children}
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  );
}

export default function App() {
  return <Outlet />;
}

export function HydrateFallback() {
  return null;
}
