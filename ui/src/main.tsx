import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/400-italic.css";
import "@fontsource/ibm-plex-sans/500.css";
import "@fontsource/ibm-plex-sans/600.css";
import "@fontsource-variable/archivo/standard.css";
import "@fontsource-variable/jetbrains-mono";
import "@/styles/globals.css";
import { applyTheme, savedTheme } from "@/lib/theme";
import "@/api/client"; // reads the token from the link before the router sees the address
import { App } from "./App";

applyTheme(savedTheme());

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
