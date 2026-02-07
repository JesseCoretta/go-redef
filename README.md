# go-redef

Package `redef` scans Go code and identifies unnecessary redefinitions of variables within functions and methods.

For example, consider the following pseudocode:

```go
func myFunction() {
	blarg, err := someFunc()
	if err != nil {
		// whatever
	}

	// ... later in the same function ...

	if err := someOtherFunc(); err != nil {
		// whatever
	}
}
```

As you can see, I needlessly redefined -- or "shadowed" -- my `err` instance inside `myFunction`.

This tool looks for, and reports on, such flaws.

## Building

This is how I currently build `redef`:

```bash
$ cd /path/to/go-redef/cmd/redef

## Note you can put redef in any folder which is monitored
## by `$PATH` -- this is simply how I do it personally, as
## I have a `bin` directory in my `$HOME`:
$ go build -o ~/bin/redef main.go
```

<sub>NOTE: In the future I'll set up `go install` for simplicity.</sub>

## Usage

`cd` to any Go package directory and run:

```bash
$ redef .
```

Alternatively, one can invoke various options, such as `--ignore-err-shadow`. See `--help` for details.

## Contributing

Please report any bugs via the Issues tab. The more eyes on this utility, the better for everyone.

## Support animal/environmental causes

If you or your organization use my software regularly and find it useful, I only ask that you donate to animal shelters, non-profit environmental entities or similar.

If you cannot afford a monetary contribution to these causes, please volunteer at animal shelters and/or visit kill shelters for the purpose of liberating animals unfairly awaiting execution.

