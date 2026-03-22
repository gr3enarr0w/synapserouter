---
base: "You MUST use tools now. Do NOT output text without tool calls. Every response must include at least one tool call."
phases:
  eda: "Start by calling file_write to create the first source file, or bash to set up the project directory."
  implement: "Start by calling file_write to create the first source file, or bash to set up the project directory."
  data-prep: "Start by calling file_write to create the first source file, or bash to set up the project directory."
  model: "Start by calling file_write to create the first source file, or bash to set up the project directory."
  self-check: "Use file_read to inspect the code, bash to run tests, and grep to check for issues."
  code-review: "Use file_read to inspect the code, bash to run tests, and grep to check for issues."
  acceptance-test: "Use file_read to inspect the code, bash to run tests, and grep to check for issues."
  review: "Use file_read to inspect the code, bash to run tests, and grep to check for issues."
  verify: "Use file_read to inspect the code, bash to run tests, and grep to check for issues."
  default: "Call bash, file_write, file_read, or any available tool."
---
