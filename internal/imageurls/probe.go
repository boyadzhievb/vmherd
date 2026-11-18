package imageurls

import (
	"fmt"
	"net/http"

	"github.com/boyadzhievb/vmherd/internal/image"
)

// singleByteRange asks a server that refuses HEAD for the first byte only, so
// checking a multi-GB image doesn't download it.
const singleByteRange = "bytes=0-0"

// probeImageURL returns the final status code of an image URL: HEAD, as
// 'vmherd pull' sends it, and a ranged GET when the server refuses HEAD
// (405 or 501).
func (checker *Checker) probeImageURL(imageURL string) (int, error) {
	statusCode, headError := image.HeadImageURL(checker.httpClient, imageURL)
	if headError != nil {
		return 0, fmt.Errorf("asking for the file: %w", headError)
	}
	if statusCode != http.StatusMethodNotAllowed && statusCode != http.StatusNotImplemented {
		return statusCode, nil
	}
	return checker.rangedGetStatusCode(imageURL)
}

// rangedGetStatusCode sends GET with a one-byte Range header and returns the
// final status code without reading the body.
func (checker *Checker) rangedGetStatusCode(imageURL string) (int, error) {
	rangedRequest, requestError := http.NewRequest(http.MethodGet, imageURL, http.NoBody) //nolint:noctx // the client's timeout bounds the request
	if requestError != nil {
		return 0, fmt.Errorf("building a ranged GET for %s: %w", imageURL, requestError)
	}
	rangedRequest.Header.Set("Range", singleByteRange)
	response, getError := checker.httpClient.Do(rangedRequest)
	if getError != nil {
		return 0, fmt.Errorf("cannot reach %s with a ranged GET: %w — check the network connection and the URL", imageURL, getError)
	}
	if closeError := response.Body.Close(); closeError != nil {
		return 0, fmt.Errorf("closing the ranged GET response from %s: %w", imageURL, closeError)
	}
	return response.StatusCode, nil
}

// isSuccessStatusCode reports whether a final status code is 2xx (a ranged
// GET answers 206).
func isSuccessStatusCode(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}
