import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// Each test starts on an empty page.
afterEach(cleanup);

// jsdom has no Tailwind. sr-only is position: absolute, which a browser
// draws as a block, so an accessible name puts a space around its words.
const style = document.createElement("style");
style.textContent = ".sr-only { display: block; }";
document.head.append(style);
