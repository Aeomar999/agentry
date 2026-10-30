import { readdirSync } from "node:fs";
import tsParser from "@typescript-eslint/parser";

// The UI's import rules between its areas, of docs/design/repo-organization.md
// (section 4, The UI):
//
// - components/ is shared by every page, so it imports neither a domain's
//   folder nor an extension's.
// - A domain imports no other domain; what two domains share moves to
//   components/ or lib/. The one exception carries a disable comment.
// - An extension imports no domain.
// - Domains and extensions import shell's leaf modules (Toasts, questions,
//   CommandPalette's types) but never AdminShell, which imports them.
// - shell/ may import any domain.
//
// Each file gets one no-restricted-imports block, since a later block for the
// same file replaces an earlier one's patterns instead of adding to them.

const domains = readdirSync(new URL("./src/domains", import.meta.url), { withFileTypes: true })
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name);

const noAdminShell = {
  regex: "^(@/|(\\.\\./)+)shell/AdminShell$",
  message: "Import shell's leaf modules (Toasts, questions), not AdminShell, which imports this area.",
};

function restrictImports(files, patterns) {
  return {
    files,
    languageOptions: { parser: tsParser },
    rules: { "no-restricted-imports": ["error", { patterns }] },
  };
}

// noOtherDomain catches another domain by its alias or a ../ path through
// domains/, and a sibling's folder reached with ../ alone.
function noOtherDomain(domain) {
  const others = domains.filter((other) => other !== domain);
  return {
    regex: `^((@/|(\\.\\./)+)domains/(?!${domain}(/|$))|(\\.\\./)+(${others.join("|")})(/|$))`,
    message: "A domain imports no other domain; move what the two share to components/ or lib/.",
  };
}

export default [
  { linterOptions: { reportUnusedDisableDirectives: "error" } },
  restrictImports(
    ["src/components/**/*.{ts,tsx}"],
    [
      {
        regex: "^(@/|(\\.\\./)+)(domains|extensions)(/|$)",
        message: "components/ imports neither domains/ nor extensions/; move this to the domain or the extension, or to shell/.",
      },
    ],
  ),
  ...domains.map((domain) => restrictImports([`src/domains/${domain}/**/*.{ts,tsx}`], [noOtherDomain(domain), noAdminShell])),
  restrictImports(
    ["src/extensions/**/*.{ts,tsx}"],
    [
      {
        regex: "^(@/|(\\.\\./)+)domains(/|$)",
        message: "An extension imports no domain; move what it needs to components/ or lib/.",
      },
      noAdminShell,
    ],
  ),
];
