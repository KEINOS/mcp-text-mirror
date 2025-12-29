# MCP text-mirror

A tiny MCP (Model Context Protocol) service written in Go. It mirrors (reverses) UTF-8 text while preserving grapheme clusters.

This repository implements a minimal MCP server and a single `mirror` tool to help me/us learn MCP basics and to build something that at minimum works with VS Code's Copilot (via `stdio` transport).

## Features

- MCP tool that reverses UTF‑8 text
- Unicode grapheme cluster–safe (handles emoji, combining marks, ZWJ sequences)
- [`stdio` transport](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports) only (HTTP/SSE transports not implemented)

## Prerequisites

- Requires Go 1.25 or newer (see [go.mod](go.mod)).
  - Go [1.25](https://go.dev/doc/devel/release#go1.25.0) is already released as of 2025-08.
- `golangci-lint` (optional; `make test` runs lint)
- `make` (optional; for build and test automation)
- MCP client that supports `stdio` transport
  - e.g., [VS Code](https://code.visualstudio.com/download) v1.102 or newer + [Copilot](https://code.visualstudio.com/docs/copilot/setup) enabled (tested with VS Code v1.107.1)

## Quick Start

This app aims to work with VS Code's Copilot extension via MCP. Any MCP-compatible client that supports `stdio` transport, Claude Desktop for example, should also work.

### Build & Test

- Build:

    ```sh
    make build
    # or
    go build -o text-mirror .
    ```

- Tests & lint:

    ```sh
    make test
    # or
    golangci-lint run --fix
    go test -cover -race ./...
    ```

### Using with VS Code (Copilot)

MCP support in VS Code is generally available as of version 1.102 and later.

1. Build the `text-mirror` as executable binary (as above) and copy its absolute path.
    - e.g., `/home/user/go/bin/text-mirror`
2. Register as an MCP server in VS Code's MCP config file.
    - Open VS Code's command palette (`F1` or `Ctrl+Shift+P`) and search for:
      - `MCP: Open User Configuration`
    - Edit the generated `mcp.json` as follows (bare minimal example):

      ```json
      {
        "servers": {
          "text-mirror": {
            "command": "/full/path/to/text-mirror",
            "env": {
              "MCP_TEXT_MIRROR_DEBUG_LOG": "/full/path/to/text-mirror.log"
            }
          }
        }
      }
      ```

      - If `mcp.json` already has other servers, just add the `text-mirror` entry under `servers`.
      - `command` is required.
        - Replace `/full/path/to/text-mirror` with the actual path to the built binary.
      - If `type` is omitted, VS Code assumes `"stdio"` by default for local servers. So no need to specify it here.
      - `env` is optional.
        - If `MCP_TEXT_MIRROR_DEBUG_LOG` is present, it enables debug logging to the specified log file.
        - Replace `/full/path/to/text-mirror.log` with the desired log file path.
      - For more details about the configuration format, see the [VS Code MCP documentation](https://code.visualstudio.com/docs/copilot/customization/mcp-servers#_configuration-format).

3. Restart VS Code
    - The `text-mirror` MCP server should appear in the VS Code extension list (as an Installed MCP server pane).
    - Click the configuration icon next to `text-mirror` and check what can be done.
      - You may but don't have to start the server manually here; VS Code will start it automatically when needed.
    - For more details, see the [VS Code MCP documentation](https://code.visualstudio.com/docs/copilot/customization/mcp-servers#_manage-installed-mcp-servers).

4. Call the `mirror` tool via Copilot Chat.
    - In Copilot Chat, select an "Agent" model that is MCP-capable.
      - If a model supports MCP tools, the “tools” icon appears.
    - The `mirror` tool should now be available. You can ask Copilot to use it:

      > Reverse this text using text-mirror: 🙂🙃🙂👩‍💻👨‍💻123

      or

      > Use the mirror tool to reverse "こんにちは"

### How It Works

If the MCP server is locally running, MCP clients like VS Code MCP/Claude Desktop communicate with it in a very Unix-like way.

1. The MCP clients start the `text-mirror` binary as a subprocess (spawns the MCP server).
2. The MCP server listens for tool calls over stdio.
3. MCP clients send instruction data (JSON-RPC) to the server’s [stdin](https://en.wikipedia.org/wiki/Standard_streams#Standard_input_(stdin)).
4. The server processes the request, reverses the text, and sends the result back via stdout.
5. The MCP client receives the response and displays it to the user.

## Development notes

- Tests with edge cases and 100% test coverage
- Linting via `golangci-lint`
- Tests include table-driven cases for combining marks, ZWJ sequences, flags, and long strings to exercise tricky Unicode behavior.
- `.editorconfig` is included to keep consistent formatting (tabs for Go files, spaces for other files, LF endings, UTF‑8 charset).

## Contributing

This project is primarily a learning resource, but contributions are welcome (fixes, tests, documentation).

## License

- MIT License. See [LICENSE](LICENSE) for details.
