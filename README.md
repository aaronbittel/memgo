# memgo

`memgo` is a small Go implementation of a memcached-like TCP server. It supports a
subset of the memcached text protocol and stores values in an in-memory,
concurrency-safe map.

## Supported Commands

- `get <key>`
- `set <key> <flags> <exptime> <bytes> [noreply]`
- `add <key> <flags> <exptime> <bytes> [noreply]`
- `replace <key> <flags> <exptime> <bytes> [noreply]`
- `append <key> <flags> <exptime> <bytes> [noreply]`
- `prepend <key> <flags> <exptime> <bytes> [noreply]`

Storage commands must be followed by a CRLF-terminated data block whose length matches
`<bytes>`.

## Run

Start the server on the default memcached port, `11211`:

```sh
go run .
```

Use a custom port:

```sh
go run . -port 11212
```

## Example

In one terminal:

```sh
go run . -port 11212
```

In another terminal:

```sh
printf 'set greeting 0 0 5\r\nhello\r\nget greeting\r\n' | nc localhost 11212
```

Expected response:

```text
STORED
VALUE greeting 0 5
hello
END
```

## Tests

Run the full test suite:

```sh
go test ./...
```

The tests cover command behavior, protocol parsing, expiration, concurrency, and store operations.

## Project Layout

- `main.go`: CLI entrypoint and startup wiring.
- `server.go`: TCP listener, connection handling, and server lifecycle.
- `commands.go`: command dispatch and command handlers.
- `protocol.go`: text protocol parsing and command metadata.
- `store.go`: in-memory store and value expiration behavior.
- `*_test.go`: unit and integration tests for the corresponding areas.
