package pdf

import (
	"fmt"
	"io"
	"os"
	"strings"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	ledpdf "github.com/ledongthuc/pdf"

	"github.com/desico/chapter-overview/internal/model"
)

const meaningfulTextThreshold = 20

// GetPageCount returns total number of pages in the PDF.
func GetPageCount(pdfPath string) (int, error) {
	return pdfcpuapi.PageCountFile(pdfPath)
}

// ExtractTOC returns PDF bookmarks/outline entries, or nil if none exist.
func ExtractTOC(pdfPath string) ([]model.ChapterBoundary, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := pdfcpumodel.NewDefaultConfiguration()
	bookmarks, err := pdfcpuapi.Bookmarks(f, conf)
	if err != nil {
		return nil, err
	}
	if len(bookmarks) == 0 {
		return nil, nil
	}

	result := make([]model.ChapterBoundary, 0, len(bookmarks))
	for _, b := range bookmarks {
		if b.PageFrom > 0 {
			result = append(result, model.ChapterBoundary{
				StartPage: b.PageFrom,
				Title:     b.Title,
			})
		}
	}
	return result, nil
}

// ExtractPageContent extracts text and embedded images from a single page.
// It opens and closes the file each call — safe for concurrent use.
func ExtractPageContent(pdfPath string, pageNum int) (model.PageContent, error) {
	content := model.PageContent{PageNumber: pageNum}

	// Extract text via ledongthuc/pdf
	text, err := extractPageText(pdfPath, pageNum)
	if err == nil {
		content.Text = text
		content.HasMeaningfulText = len(strings.TrimSpace(text)) >= meaningfulTextThreshold
	}

	// Extract embedded images via pdfcpu
	images, err := extractPageImages(pdfPath, pageNum)
	if err == nil {
		content.Images = images
	}

	return content, nil
}

// ExtractPagesRange extracts content for pages [startPage, endPage] inclusive.
// Opens the PDF once for text extraction across the range.
func ExtractPagesRange(pdfPath string, startPage, endPage int) ([]model.PageContent, error) {
	results := make([]model.PageContent, 0, endPage-startPage+1)
	for page := startPage; page <= endPage; page++ {
		content, err := ExtractPageContent(pdfPath, page)
		if err != nil {
			content = model.PageContent{PageNumber: page}
		}
		results = append(results, content)
	}
	return results, nil
}

// SplitPDF extracts pages [startPage, endPage] inclusive into outputPath.
func SplitPDF(pdfPath string, startPage, endPage int, outputPath string) error {
	pageRange := fmt.Sprintf("%d-%d", startPage, endPage)
	conf := pdfcpumodel.NewDefaultConfiguration()
	return pdfcpuapi.CollectFile(pdfPath, outputPath, []string{pageRange}, conf)
}

// extractPageText uses ledongthuc/pdf to get plain text from one page.
func extractPageText(pdfPath string, pageNum int) (string, error) {
	f, r, err := ledpdf.Open(pdfPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if pageNum < 1 || pageNum > r.NumPage() {
		return "", fmt.Errorf("page %d out of range (1-%d)", pageNum, r.NumPage())
	}

	page := r.Page(pageNum)
	if page.V.IsNull() {
		return "", nil
	}

	var sb strings.Builder
	rows, err := page.GetTextByRow()
	if err != nil {
		// Fallback: try plain text
		plain, e2 := page.GetPlainText(nil)
		if e2 != nil {
			return "", err
		}
		return plain, nil
	}
	for _, row := range rows {
		for _, word := range row.Content {
			sb.WriteString(word.S)
			sb.WriteByte(' ')
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// supportedImageMIME maps pdfcpu FileType to MIME types supported by vision APIs.
var supportedImageMIME = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"gif":  "image/gif",
}

// extractPageImages extracts embedded images from a specific page via pdfcpu.
// Only images in formats supported by vision LLM APIs are returned.
func extractPageImages(pdfPath string, pageNum int) ([][]byte, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	conf := pdfcpumodel.NewDefaultConfiguration()
	pageStr := fmt.Sprintf("%d", pageNum)

	var imageData [][]byte
	err = pdfcpuapi.ExtractImages(f, []string{pageStr}, func(img pdfcpumodel.Image, singlePage bool, pageNr int) error {
		ft := strings.ToLower(img.FileType)
		if _, ok := supportedImageMIME[ft]; !ok {
			return nil // skip unsupported formats (e.g. jpx/jpeg2000)
		}
		data, readErr := io.ReadAll(img)
		if readErr != nil {
			return nil
		}
		imageData = append(imageData, data)
		return nil
	}, conf)

	if err != nil {
		return nil, err
	}
	return imageData, nil
}
