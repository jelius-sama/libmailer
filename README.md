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

The library can automatically loads configuration from:

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

```c
#include "libmailer/libmailer.h"
```

And link statically:

```sh
gcc main.c -L./libmailer -lmailer -o myprog
```

---

## C API Overview

All C API functions follow standard C idioms:

* Functions return `int` status codes (`0` for success, `-1` for error)
* Output values are returned via pointer parameters
* Error messages are returned via `char**` output parameters
* Always check return codes and free allocated memory

---

### Load configuration

```c
MailerConfig *config = NULL;
char *error = NULL;

// Load configuration from default location
int status = LoadConfig(&config, &error);
if (status != 0) {
    fprintf(stderr, "error: %s\n", error);
    FreeCString(error);
    return 1;
}

// Use config...
FreeMailerConfig(config);
```

Or load from an explicit path:

```c
MailerConfig *config = NULL;
char *error = NULL;

int status = LoadConfigFromPath("/path/to/config.json", &config, &error);
if (status != 0) {
    fprintf(stderr, "error: %s\n", error);
    FreeCString(error);
    return 1;
}

// Use config...
FreeMailerConfig(config);
```

### Free the config

```c
FreeMailerConfig(config);
```

---

### Parse an address

```c
char *parsed = NULL;
char *error = NULL;

int status = ParseEmailAddress("Name <test@example.com>", &parsed, &error);
if (status != 0) {
    fprintf(stderr, "parse error: %s\n", error);
    FreeCString(error);
} else {
    printf("parsed: %s\n", parsed);
    FreeCString(parsed);
}
```

### Format an address

```c
char *formatted = NULL;
FormatEmailAddress("test@example.com", &formatted);
printf("formatted: %s\n", formatted);
FreeCString(formatted);
```

---

### Sending mail

```c
char *error = NULL;

int status = SendMail(
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
    NULL,      // Attachments (StrArr*)
    &error
);

if (status != 0) {
    fprintf(stderr, "send error: %s\n", error);
    FreeCString(error);
    return 1;
}

printf("Mail sent successfully\n");
```

### Sending a raw `.eml`

```c
char *error = NULL;

int status = SendRawEML(
    "smtp.example.com",
    587,
    "user@example.com",
    "pass123",
    "message.eml",
    &error
);

if (status != 0) {
    fprintf(stderr, "send error: %s\n", error);
    FreeCString(error);
    return 1;
}

printf("EML sent successfully\n");
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

Example of creating a StrArr with CC recipients:

```c
// Allocate array of char pointers
char **cc_array = malloc(2 * sizeof(char*));
cc_array[0] = strdup("cc1@example.com");
cc_array[1] = strdup("cc2@example.com");

// Create StrArr wrapper
StrArr *cc = malloc(sizeof(StrArr));
cc->str = (char*)cc_array;  // pointer to first element
cc->count = 2;
cc->len = 2 * sizeof(char*);

// Use in SendMail
char *error = NULL;
int status = SendMail(
    host, port, username, password,
    from, to, subject, body,
    cc,    // CC recipients
    NULL,  // BCC
    NULL,  // Attachments
    &error
);

// Clean up
FreeStrArr(cc);
```

The library provides `FreeStrArr` to properly deallocate a StrArr and all its contained strings.

---

## Memory Management

**Important**: The C API allocates memory that must be freed by the caller:

* `FreeCString(char*)` - Free individual strings returned by functions
* `FreeMailerConfig(MailerConfig*)` - Free configuration structures
* `FreeStrArr(StrArr*)` - Free string array structures

Always free allocated memory to prevent leaks.

---

## Error Handling Pattern

All functions that can fail follow this pattern:

```c
OutputType *output = NULL;
char *error = NULL;

int status = FunctionName(inputs..., &output, &error);
if (status != 0) {
    // Handle error
    fprintf(stderr, "Error: %s\n", error);
    FreeCString(error);
    return status;
}

// Success - use output
// ...

// Clean up
FreeOutputType(output);
```

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
