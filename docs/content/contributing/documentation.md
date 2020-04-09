# Documentation

You've found something unclear in the documentation and want to give a try at explaining it better?
Let's see how.

## Building

This [documentation](https://docs.mae.sh/) is built with [mkdocs](https://mkdocs.org/).

### With `Docker` and `make`

You can build the documentation and test it locally (with live reloading), using the `serve` target:

```bash
$ make serve
docker build -t maesh-docs -f docs.Dockerfile ./
# […]
docker run  --rm -v /Users/kevinpollet/Documents/Dev/maesh/docs:/mkdocs  -p 8000:8000 maesh-docs mkdocs serve
# […]
INFO    -  Building documentation...
INFO    -  Cleaning site directory
[I 200408 14:36:33 server:296] Serving on http://0.0.0.0:8000
[I 200408 14:36:33 handlers:62] Start watching changes
[I 200408 14:36:33 handlers:64] Start detecting changes
```

!!! tip "Default URL"
    By default the local documentation server listens on [http://127.0.0.1:8000](http://127.0.0.1:8000).

If you only want to build the documentation without serving it locally, you can use the `build` target.

### With `mkdocs`

First, make sure you have `python` and `pip` installed.

```bash
$ python --version
Python 2.7.2

$ pip --version
pip 1.5.2
```

Then, install mkdocs with `pip`.

```bash
pip install --user -r requirements.txt
```

To build the documentation and serve it locally, run `mkdocs serve` from the root directory.
This will start a local server:

```bash
$ mkdocs serve
INFO    -  Building documentation...
INFO    -  Cleaning site directory
[I 160505 22:31:24 server:281] Serving on http://127.0.0.1:8000
[I 160505 22:31:24 handlers:59] Start watching changes
[I 160505 22:31:24 handlers:61] Start detecting changes
```

## Checking

To check that the documentation meets standard expectations (no dead links, html markup validity, ...), use the `verify` target.
If you've made changes to the documentation, it's safer to clean it before verifying it.

```bash
$ make clean verify
docker build -t maesh-docs -f docs.Dockerfile ./
# […]
docker run --rm -v /Users/kevinpollet/Documents/Dev/maesh/docs:/mkdocs  -p 8000:8000 maesh-docs sh -c "mkdocs build && chown -R 501:20 ./site"
=== Checking HTML content...
# […]
```

!!! Note "Disabling Verification"
    Verification can be disabled by setting the environment variable `DOCS_VERIFY_SKIP` to `true`:
    
    ```bash
    $ DOCS_VERIFY_SKIP=true make verify
    # […]
    DOCS_VERIFY_SKIP is true: no verification done.
    ```
