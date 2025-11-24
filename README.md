# libmailer

A small interface layer around a mail-sending API, usable from both Go and plain C.
The library provides high-level conveniences when used from Go, and a thin exported C ABI for low-level native integration.

It supports:

* Loading mail configuration (`host`, `port`, credentials, sender)
* Address parsing and formatting
* Sending regular text/HTML mail
* Sending raw `.eml` files
* Optional CC / BCC / attachments

The project exposes:

* A Go package at:
  `import ( libmailer "github.com/jelius-sama/libmailer/api" )`
* A C API via FFI:
  Header: `libmailer.h`
  Static archive: `libmailer.a`

---

## Project Layout

```
.
├── api/              # Go API package
│   └── api.go
├── libmailer.go      # C FFI layer (exported symbols)
├── libmailer.h       # Generated when running `make`
├── libmailer.a       # Generated static library
├── Makefile
├── go.mod
├── go.sum
└── LICENSE
```

---

# Go Usage

## Installation

```sh
go get github.com/jelius-sama/libmailer
```

Import the API:

```go
import (
    libmailer "github.com/jelius-sama/libmailer/api"
)
```

---

## Configuration

The library automatically loads configuration from:

```
~/.config/mailer/config.json
```

Example config:

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "user@example.com",
  "password": "supersecret",
  "from": "User <user@example.com>"
}
```

You can also load manually:

```go
cfg, err := libmailer.LoadConfigFromPath("/path/to/config.json")
```

---

## Sending a Simple Mail

```go
package main

import (
    "log"
    libmailer "github.com/jelius-sama/libmailer/api"
)

func main() {
    cfg, err := libmailer.LoadConfig()
    if err != nil {
        log.Fatal(err)
    }

    err = libmailer.SendMail(
        cfg.Host,
        cfg.Port,
        cfg.Username,
        cfg.Password,
        cfg.From,
        "receiver@example.com",
        "Hello World",
        "This is a plain text message.",
        nil,
        nil,
        nil,
    )

    if err != nil {
        log.Fatal(err)
    }
}
```

---

## Sending a Raw `.eml`

```go
err := libmailer.SendRawEML(cfg.Host, cfg.Port, cfg.Username, cfg.Password, "message.eml")
```

---

## Email Address Utilities

```go
parsed, err := libmailer.ParseEmailAddress("Name <someone@example.com>")
fmt.Println(parsed) // someone@example.com

formatted := libmailer.FormatEmailAddress("someone@example.com")
fmt.Println(formatted) // someone@example.com
```

---

# C Usage (FFI)

The native layer is implemented through `libmailer.go` and built using the provided Makefile.

## Building the C Library

Inside the project directory:

```sh
make
```

This will generate:

* `libmailer.h`  (header)
* `libmailer.a`  (static library)

Use them inside your C project:

```
#include "libmailer/libmailer.h"
```

And link statically:

```sh
gcc main.c -L./libmailer -lmailer -o myprog
```

---

## C API Overview

### Load configuration

```c
MailerConfig* cnf = NULL;
char* err = NULL;

// Load configuration
struct LoadConfig_return ret = LoadConfig();
cnf = ret.r0; err = ret.r1;

if (err != NULL) {
    fprintf(stderr, "error: %s\n", err);
    FreeCString(err);
    return 1;
}
```

Or load from an explicit path:

```c
struct LoadConfigFromPath_return ret = LoadConfigFromPath("/path/to/config.json");
```

### Free the config

```c
FreeMailerConfig(cfg);
```

---

### Parse an address

```c
char *out;
char *err;

struct ParseEmailAddress_return ret = ParseEmailAddress("Name <test@example.com>");
out = ret.r0; err = ret.r1;

if (err != NULL) {
    fprintf(stderr, "%s\n", err);
    FreeCString(err);
} else {
    printf("parsed: %s\n", out);
    FreeCString(out);
}
```

### Format an address

```c
char *out = FormatEmailAddress("test@example.com");
printf("formatted: %s\n", out);
FreeCString(out);
```

---

### Sending mail

```c
char *err = SendMail(
    "smtp.example.com",
    587,
    "user@example.com",
    "pass123",
    "sender@example.com",
    "receiver@example.com",
    "Subject",
    "Body text",
    NULL,      // CC (StrArr*)
    NULL,      // BCC (StrArr*)
    NULL       // Attachments (StrArr*)
);

if (err != NULL) {
    fprintf(stderr, "send error: %s\n", err);
    FreeCString(err);
}
```

### Sending a raw `.eml`

```c
char *err = SendRawEML(
    "smtp.example.com",
    587,
    "user@example.com",
    "pass123",
    "message.eml"
);
```

---

## StrArr (CC, BCC, Attachments)

If you need CC, BCC, or attachments, you must construct a `StrArr`.

The structure:

```c
typedef struct {
    char *str;     // pointer to first char* element
    size_t len;    // total buffer length
    size_t count;  // number of strings
} StrArr;
```

Allocate and free manually as normal C memory.
The library provides `FreeStrArr` to release one created on your side.

---

# Build From Source

```sh
make
```

This compiles the FFI exports and produces the static library.

---

# License

See `LICENSE` for details.

---
