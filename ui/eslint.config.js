import tsParser from "@typescript-eslint/parser";

// The one UI import rule of docs/design/repo-organization.md (section 4,
// The UI): components/ is shared by every page, so it imports neither a
// domain's folder nor an extension's.
export default [
  {
    files: ["src/components/**/*.{ts,tsx}"],
    languageOptions: { parser: tsParser },
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              regex: "^(@/|(\\.\\./)+)(domains|extensions)(/|$)",
              message: "components/ imports neither domains/ nor extensions/; move this to the domain or the extension, or to shell/.",
            },
          ],
        },
      ],
    },
  },
];
