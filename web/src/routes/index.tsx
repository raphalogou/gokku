import { redirect } from "react-router";

// Projects are the home screen.
export function clientLoader() {
  throw redirect("/projects");
}

export default function Index() {
  return null;
}
