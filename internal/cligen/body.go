package cligen

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type bodyFlags struct {
	required bool

	supportedContentTypes []string

	data        *string
	contentType *string
	form        *[]string
}

func bindBodyFlags(cmd *cobra.Command, required bool, supportedContentTypes []string) *bodyFlags {
	b := &bodyFlags{
		required:              required,
		supportedContentTypes: append([]string(nil), supportedContentTypes...),
		data:                  new(string),
		contentType:           new(string),
		form:                  new([]string),
	}

	cmd.Flags().StringVar(b.data, "data", "", "Request body data: '@file.json', '-' for stdin, or inline string")
	cmd.Flags().StringVar(b.contentType, "content-type", "", "Override request Content-Type")
	cmd.Flags().StringArrayVar(b.form, "form", nil, "Form field: key=value or key=@file (repeatable)")

	return b
}

func (b *bodyFlags) build(cmd *cobra.Command) (body []byte, contentType string, err error) {
	if b == nil || cmd == nil {
		return nil, "", nil
	}

	dataChanged := cmd.Flags().Changed("data")
	formChanged := cmd.Flags().Changed("form")
	ctChanged := cmd.Flags().Changed("content-type")

	selectedCT := ""
	if ctChanged {
		selectedCT = strings.TrimSpace(*b.contentType)
		if selectedCT == "" {
			return nil, "", fmt.Errorf("--content-type set but empty")
		}
		if !b.supportsContentType(selectedCT) {
			return nil, "", fmt.Errorf("unsupported --content-type %q for this operation", selectedCT)
		}
	}

	hasForm := formChanged && len(*b.form) > 0
	hasData := dataChanged && strings.TrimSpace(*b.data) != ""

	if b.required && !hasForm && !hasData {
		return nil, "", fmt.Errorf("request body required; provide --data or --form")
	}
	if !hasForm && !hasData {
		return nil, "", nil
	}

	if hasForm {
		if selectedCT == "" {
			selectedCT = b.defaultFormContentType()
			if selectedCT == "" {
				return nil, "", fmt.Errorf("this operation does not support form bodies; use --data")
			}
		}
	}
	if hasData {
		if selectedCT == "" {
			selectedCT = b.defaultDataContentType()
			if selectedCT == "" {
				return nil, "", fmt.Errorf("unable to pick a request content-type for this operation")
			}
		}
	}

	switch {
	case strings.HasPrefix(selectedCT, "application/json"):
		if !hasData {
			return nil, "", fmt.Errorf("JSON request body requires --data")
		}
		raw, err := readDataArg(*b.data)
		if err != nil {
			return nil, "", err
		}
		return raw, selectedCT, nil

	case strings.HasPrefix(selectedCT, "application/x-www-form-urlencoded"):
		if !hasForm {
			return nil, "", fmt.Errorf("form-encoded body requires --form")
		}
		vals := url.Values{}
		for _, entry := range *b.form {
			k, v, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(k) == "" {
				return nil, "", fmt.Errorf("invalid --form %q (expected key=value)", entry)
			}
			if strings.HasPrefix(v, "@") {
				return nil, "", fmt.Errorf("file upload not supported for application/x-www-form-urlencoded: %q", entry)
			}
			vals.Add(k, v)
		}
		return []byte(vals.Encode()), "application/x-www-form-urlencoded", nil

	case strings.HasPrefix(selectedCT, "multipart/form-data"):
		if !hasForm {
			return nil, "", fmt.Errorf("multipart body requires --form")
		}
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for _, entry := range *b.form {
			k, v, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(k) == "" {
				_ = w.Close()
				return nil, "", fmt.Errorf("invalid --form %q (expected key=value)", entry)
			}
			if strings.HasPrefix(v, "@") {
				path := strings.TrimPrefix(v, "@")
				f, err := os.Open(path)
				if err != nil {
					_ = w.Close()
					return nil, "", err
				}
				part, err := w.CreateFormFile(k, filepath.Base(path))
				if err != nil {
					_ = f.Close()
					_ = w.Close()
					return nil, "", err
				}
				if _, err := io.Copy(part, f); err != nil {
					_ = f.Close()
					_ = w.Close()
					return nil, "", err
				}
				_ = f.Close()
			} else {
				if err := w.WriteField(k, v); err != nil {
					_ = w.Close()
					return nil, "", err
				}
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), w.FormDataContentType(), nil

	default:
		return nil, "", fmt.Errorf("unsupported content-type %q", selectedCT)
	}
}

func readDataArg(arg string) ([]byte, error) {
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "-":
		return io.ReadAll(os.Stdin)
	case strings.HasPrefix(arg, "@"):
		return os.ReadFile(strings.TrimPrefix(arg, "@"))
	default:
		return []byte(arg), nil
	}
}

func (b *bodyFlags) supportsContentType(ct string) bool {
	for _, s := range b.supportedContentTypes {
		if s == ct {
			return true
		}
		// Allow users to specify "application/json" when spec uses "application/json;charset=utf-8".
		if strings.HasPrefix(s, ct) && strings.HasPrefix(s, "application/json") && strings.HasPrefix(ct, "application/json") {
			return true
		}
	}
	return false
}

func (b *bodyFlags) defaultDataContentType() string {
	for _, ct := range b.supportedContentTypes {
		if strings.HasPrefix(ct, "application/json") {
			return ct
		}
	}
	if len(b.supportedContentTypes) > 0 {
		return b.supportedContentTypes[0]
	}
	return ""
}

func (b *bodyFlags) defaultFormContentType() string {
	for _, ct := range b.supportedContentTypes {
		if strings.HasPrefix(ct, "multipart/form-data") {
			return ct
		}
	}
	for _, ct := range b.supportedContentTypes {
		if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			return ct
		}
	}
	return ""
}
