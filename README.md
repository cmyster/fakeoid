# fakeoid

A tool of sorts that would use several agents to acheive simple coding tasks.

## Requirments
I am using this for my own hardware, so this tool expects AMD GPU, ROCm and since this tool would also compile and test code, whichever build chains that you need to have.
It was build using Go and uses llama-server and git.

## Usage
  1. Build it
  clone and cd.
  go build -o fakeoid ./cmd/fakeoid/

  2. Check prerequisites
  ./fakeoid check
  This verifies ROCm, llama-server, Go, and Git are available.

  3. Download a model (if you haven't already)
  ./fakeoid download
  Auto-fetches a GGUF model.

  4. Run it
  cd /some/test/project   # any Go project directory
  path/fakeoid/fakeoid

  This starts llama-server, shows the interactive prompt, and activates Agent 1.

  5. Give it a task

  Describe something simple to Agent 1, like:
  ▎ Create a cli application in Zig that prints the current running kernel version.

  * Agent 1 initializes the request.
  * Agent 2 works to streamline the request so an LLM can understand it better.
  * Agent 3 creates the plan and folder/file structure for the project.
  * Agent 4 generates the code.
  * Agent 5 tries to generate automatic tests.
  * Agent 6 checks that a final product works and acts as designed.

  Between agents 4 and 6 a loop would start with fixes and changes as long as agents 5 and 6 are not happy.
