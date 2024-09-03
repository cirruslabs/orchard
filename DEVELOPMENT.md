## Development

Development is done as one would normally develop any Golang package, however, if you did modify any `*.proto` files in the `rpc/` directory, install [Buf](https://buf.build/) and run the following command from the project's root directory to re-generate the code:

```shell
buf generate
```
