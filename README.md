# fakeoid

A tool of sorts that would use several agents to acheive simple coding tasks.

This is not a replacemnt for professional payed tools which can do the same but fast.

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
  Auto-fetches default GGUF model, currently its google_gemma-4-31B-it-Q4_K_M.gguf

  5. Run it
  cd /some/test/project
  path/fakeoid/fakeoid

  This starts llama-server, shows the interactive prompt, and activates Agent 1.

  5. Give it a task.

  Describe something simple to Agent 1, like:
  
  ▎ Create a cli application in Zig that prints the current running kernel version.

  * Agent 1 initializes the request.
  * Agent 2 works to streamline the request so an LLM can understand it better.
  * Agent 3 creates the plan and folder/file structure for the project.
  * Agent 4 generates the code.
  * Agent 5 breakes down testing tasks.
  * Agent 6.1 knows what the tool does but does not see the code - blackbox testing.
  * Agent 6.2 doesn't know what the tool does and only sees code - whitebox testing.
  * Agent 7 verifies that the end result is inline with the task.

  ## Verification process
  Between agents 4 and 7 a loop would start (configurable loop amount). Agent 7 will make sure that not only all tests are passing, but that the code is as defined in the original task:
  * Agent  4 will make sure that the code is compiling and rewrite if not.
  * Agents 6.N will each make sure that their respective tests are passing and returnm to agent 4 otherwise with issues.
  * Agent  7 will check for allignment that the end result matches the original task.
  
