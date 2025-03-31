# Library libcopyoffload

The copy-offload API allows a user's compute application to specify data movement requests to the [copy-offload server](https://nearnodeflash.github.io/dev/guides/data-movement/copy-offload/). The user's application utilizes the `libcopyoffload` library to establish a secure connection to the copy-offload server to initiate, list, query the status of, or cancel data movement requests.

See [copy-offload.h](./copy-offload.h) for the API description. See the [test tool](./test-tool/main.c) for examples of using the API.

The build environment requires libraries and headers for `libcurl` and `json-c`. On a Mac run `brew install json-c`, and on a Redhat Linux machine look for [json-c rpms](https://www.rpmfind.net/linux/rpm2html/search.php?query=json-c).
