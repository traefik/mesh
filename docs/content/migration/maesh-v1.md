# Minor Migrations

Maesh v1
{: .subtitle }

## v1.1 to v1.2

### Debug

The `--debug` CLI flag is deprecated and will be removed in a future major release. Instead, you should use the new 
`--logLevel` flag with `debug` as value.

### SMI Mode

The `--smi` CLI flag is deprecated and will be removed in a future major release. Instead, you should use the new and 
backward compatible `--acl` flag.
