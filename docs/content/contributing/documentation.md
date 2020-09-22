# Documentation

You've found something unclear in the documentation and want to give a try at explaining it better?
Let's see how.

## Building

This [documentation](https://doc.traefik.io/traefik-mesh/) is built with [MkDocs](https://mkdocs.org/).

### With `Docker` and `make`

You can build the documentation and test it locally (with live reloading), using the `serve` target:

```bash
$ make serve
docker build -t traefik-mesh-docs -f docs.Dockerfile ./
# […]
docker run  --rm -v /home/user/traefik-mesh/docs:/mkdocs  -p 8000:8000 traefik-mesh-docs mkdocs serve
# […]
INFO    -  Building documentation...
INFO    -  Cleaning site directory
[I 200408 14:36:33 server:296] Serving on http://0.0.0.0:8000
[I 200408 14:36:33 handlers:62] Start watching changes
[I 200408 14:36:33 handlers:64] Start detecting changes
```

!!! Note
    By default, the local documentation server listens on [http://127.0.0.1:8000](http://127.0.0.1:8000).
    To build the documentation without serving it locally, use the `build` target.

### With `MkDocs`

First, make sure you have `python` and `pip` installed. MkDocs supports `python` versions `2.7.9+`, `3.4`, `3.5`, `3.6` 
and `3.7`.

```bash
$ python --version
Python 2.7.14

$ pip --version
pip 19.3.1 from /usr/local/lib/python2.7/site-packages/pip (python 2.7)
```

Then, install MkDocs with `pip`.

```bash
pip install --user -r requirements.txt
```

To build the documentation and serve it locally, run `mkdocs serve` from the root directory.
This starts a local server, and exposes the documentation on `http://127.0.0.1:8000`:

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
docker build -t traefik-mesh-docs -f docs.Dockerfile ./
# […]
docker run --rm -v /home/user/traefik-mesh/docs:/mkdocs  -p 8000:8000 traefik-mesh-docs sh -c "mkdocs build && chown -R 501:20 ./site"
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
