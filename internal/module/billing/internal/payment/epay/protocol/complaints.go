package protocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
)

// UploadComplaintImage uploads one PNG/JPEG (SDK limit: 5 MiB). File bytes are
// excluded from the protocol signature; HTTPS remains mandatory.
func (c *Client) UploadComplaintImage(ctx context.Context, complaintID string, data []byte) (string, error) {
	if complaintID == "" || len(data) == 0 || len(data) > 5<<20 {
		return "", errors.New("epay: complaint ID and a nonempty image up to 5 MiB are required")
	}
	name := "evidence.jpg"
	switch http.DetectContentType(data) {
	case "image/png":
		name = "evidence.png"
	case "image/jpeg":
	default:
		return "", errors.New("epay: complaint image must be PNG or JPEG")
	}
	form, err := c.form(Request{Params: Params{"id": complaintID}})
	if err != nil {
		return "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, values := range form {
		if err := writer.WriteField(key, values[0]); err != nil {
			return "", err
		}
	}
	file, err := writer.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	response, err := c.send(ctx, c.baseURL+"/api/complain/upload", writer.FormDataContentType(), &body)
	if err != nil {
		return "", err
	}
	var result struct {
		ImageID string `json:"image_id"`
	}
	if err := response.decodeRequired(&result, "image_id"); err != nil {
		return "", err
	}
	return result.ImageID, nil
}
