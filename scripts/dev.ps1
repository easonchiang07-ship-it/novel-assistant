param(
    [Parameter(Position = 0)]
    [ValidateSet("fmt", "test", "build", "run", "tidy")]
    [string]$Task = "test"
)

$ErrorActionPreference = "Stop"

switch ($Task) {
    "fmt" {
        gofmt -w cmd internal
    }
    "test" {
        go test ./...
    }
    "build" {
        go build ./...
    }
    "run" {
        go run ./cmd
    }
    "tidy" {
        go mod tidy
    }
}
