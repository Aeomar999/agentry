import { Lightbulb } from "lucide-react";
import type { ExtensionUI } from "..";
import { IdeasPage } from "./IdeasPage";
import { TurnOnIdeas } from "./AskVisionaries";

export const ideas: ExtensionUI = {
  name: "ideas",
  label: "Ideas",
  routes: [{ path: "/ideas", element: <IdeasPage /> }],
  nav: [{ section: "work", to: "/ideas", label: "Ideas", icon: <Lightbulb /> }],
  streams: { prefix: "idea-", to: (stream) => `/ideas#${stream}` },
  commands: (navigate) => [{ id: "go-ideas", group: "Go to", label: "Ideas", icon: <Lightbulb />, run: () => navigate("/ideas") }],
  turnOn: TurnOnIdeas,
};
