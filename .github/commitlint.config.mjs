// Commitlint config — enforces Conventional Commits as defined in CONTRIBUTING.md
export default {
  extends: ["@commitlint/config-conventional"],
  rules: {
    // Types allowed per CONTRIBUTING.md
    "type-enum": [
      2,
      "always",
      ["feat", "fix", "docs", "style", "refactor", "test", "chore"],
    ],
    // Max 72 chars in subject line
    "header-max-length": [2, "always", 72],
    // Type must be lowercase
    "type-case": [2, "always", "lower-case"],
    // Subject must not end with period
    "subject-full-stop": [2, "never", "."],
  },
};
