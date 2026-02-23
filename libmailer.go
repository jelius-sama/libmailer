package main

/*
#include <stdlib.h>

// MailerConfig reflects api.Config for the AWS SES branch.
// SMTP credentials (Host, Port, Username, Password) are intentionally absent —
// AWS authentication is handled by the SDK credential chain (environment
// variables, ~/.aws/credentials, EC2/ECS instance/task IAM roles, etc.).
typedef struct {
    char *From;
    char *Region;
    int   UseDualStack; // 0 = false, 1 = true
} MailerConfig;

typedef struct {
    char   *str;
    size_t  len;
    size_t  count;
} StrArr;
*/
import "C"

import (
    "github.com/jelius-sama/libmailer/api"
    "unsafe"
)

// strArrToSlice converts a C StrArr (array of char*) to a Go []string.
func strArrToSlice(arr *C.StrArr) []string {
    if arr == nil || arr.count == 0 {
        return nil
    }

    length := int(arr.count)
    goSlice := make([]string, length)

    ptr := unsafe.Pointer(arr.str)
    size := unsafe.Sizeof(*arr.str)

    for i := range length {
        cstr := *(**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*size))
        goSlice[i] = C.GoString(cstr)
    }

    return goSlice
}

// configToC allocates a C MailerConfig from a Go api.Config and returns it.
// The caller is responsible for freeing it with FreeMailerConfig.
func configToC(cnf *api.Config) *C.MailerConfig {
    mailerConfig := (*C.MailerConfig)(C.malloc(C.size_t(unsafe.Sizeof(C.MailerConfig{}))))
    mailerConfig.From = C.CString(cnf.From)
    mailerConfig.Region = C.CString(cnf.Region)
    if cnf.UseDualStack {
        mailerConfig.UseDualStack = 1
    } else {
        mailerConfig.UseDualStack = 0
    }
    return mailerConfig
}

//export FreeCString
func FreeCString(cstr *C.char) {
    if cstr != nil {
        C.free(unsafe.Pointer(cstr))
    }
}

// LoadConfig loads AWS SES configuration from the default path
// (~/.config/mailer/config.aws.json).
//
//export LoadConfig
func LoadConfig(out_config **C.MailerConfig, out_error **C.char) C.int {
    cnf, err := api.LoadConfig()
    if err != nil {
        *out_error = C.CString(err.Error())
        *out_config = nil
        return -1
    }

    *out_config = configToC(cnf)
    *out_error = nil
    return 0
}

// LoadConfigFromPath loads AWS SES configuration from the given file path.
//
//export LoadConfigFromPath
func LoadConfigFromPath(configPath *C.char, out_config **C.MailerConfig, out_error **C.char) C.int {
    cnf, err := api.LoadConfigFromPath(C.GoString(configPath))
    if err != nil {
        *out_error = C.CString(err.Error())
        *out_config = nil
        return -1
    }

    *out_config = configToC(cnf)
    *out_error = nil
    return 0
}

// FreeMailerConfig frees a MailerConfig struct and all strings it contains.
//
//export FreeMailerConfig
func FreeMailerConfig(cfg *C.MailerConfig) {
    if cfg == nil {
        return
    }

    C.free(unsafe.Pointer(cfg.From))
    C.free(unsafe.Pointer(cfg.Region))
    C.free(unsafe.Pointer(cfg))
}

// ParseEmailAddress validates and normalises an email address, returning just
// the address portion (strips any display name).
//
//export ParseEmailAddress
func ParseEmailAddress(addr *C.char, out_parsed **C.char, out_error **C.char) C.int {
    parsed, err := api.ParseEmailAddress(C.GoString(addr))
    if err != nil {
        *out_error = C.CString(err.Error())
        *out_parsed = nil
        return -1
    }

    *out_parsed = C.CString(parsed)
    *out_error = nil
    return 0
}

// FormatEmailAddress returns the RFC 5322 formatted version of an address,
// e.g. "Display Name <user@example.com>".
//
//export FormatEmailAddress
func FormatEmailAddress(addr *C.char, out_formatted **C.char) {
    *out_formatted = C.CString(api.FormatEmailAddress(C.GoString(addr)))
}

// SendMail composes and sends an email via AWS SES v2.
// cfg must be a valid MailerConfig obtained from LoadConfig or LoadConfigFromPath.
// Returns 0 on success, -1 on error (out_error is set and must be freed with
// FreeCString).
//
//export SendMail
func SendMail(cfg *C.MailerConfig, from, to, subject, body *C.char, cc, bcc, attachments *C.StrArr, out_error **C.char) C.int {
    if cfg == nil {
        *out_error = C.CString("cfg must not be nil")
        return -1
    }

    goCfg := &api.Config{
        From:         C.GoString(cfg.From),
        Region:       C.GoString(cfg.Region),
        UseDualStack: cfg.UseDualStack != 0,
    }

    err := api.SendMail(
        goCfg,
        C.GoString(from),
        C.GoString(to),
        C.GoString(subject),
        C.GoString(body),
        strArrToSlice(cc),
        strArrToSlice(bcc),
        strArrToSlice(attachments),
    )
    if err != nil {
        *out_error = C.CString(err.Error())
        return -1
    }

    *out_error = nil
    return 0
}

// FreeStrArr frees a StrArr struct and all strings it contains.
//
//export FreeStrArr
func FreeStrArr(arr *C.StrArr) {
    if arr == nil {
        return
    }

    ptr := unsafe.Pointer(arr.str)
    size := unsafe.Sizeof(*arr.str)
    for i := range int(arr.count) {
        cstr := *(**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*size))
        if cstr != nil {
            C.free(unsafe.Pointer(cstr))
        }
    }

    C.free(unsafe.Pointer(arr.str))
    C.free(unsafe.Pointer(arr))
}

// SendRawEML sends a pre-composed .eml file via AWS SES v2.
// The file bytes are forwarded verbatim — headers, encoding and MIME structure
// are fully preserved.
// cfg must be a valid MailerConfig obtained from LoadConfig or LoadConfigFromPath.
// Returns 0 on success, -1 on error (out_error is set and must be freed with
// FreeCString).
//
//export SendRawEML
func SendRawEML(cfg *C.MailerConfig, emlPath *C.char, out_error **C.char) C.int {
    if cfg == nil {
        *out_error = C.CString("cfg must not be nil")
        return -1
    }

    goCfg := &api.Config{
        From:         C.GoString(cfg.From),
        Region:       C.GoString(cfg.Region),
        UseDualStack: cfg.UseDualStack != 0,
    }

    err := api.SendRawEML(goCfg, C.GoString(emlPath))
    if err != nil {
        *out_error = C.CString(err.Error())
        return -1
    }

    *out_error = nil
    return 0
}

func main() {}

