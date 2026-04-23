package pdf

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

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

// ExtractPagesRange extracts content for pages [startPage, endPage] inclusive.
// Opens the PDF once for text extraction and once for image extraction across the range.
func ExtractPagesRange(pdfPath string, startPage, endPage int) ([]model.PageContent, error) {
	// Open ledpdf once for all text in range
	f, r, err := ledpdf.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}
	defer f.Close()

	// Batch image extraction via pdfcpu using page range string
	imgByPage := make(map[int][][]byte)
	if pdfFile, e := os.Open(pdfPath); e == nil {
		conf := pdfcpumodel.NewDefaultConfiguration()
		pageRange := fmt.Sprintf("%d-%d", startPage, endPage)
		_ = pdfcpuapi.ExtractImages(pdfFile, []string{pageRange}, func(img pdfcpumodel.Image, _ bool, pageNr int) error {
			ft := strings.ToLower(img.FileType)
			if _, ok := supportedImageMIME[ft]; !ok {
				return nil
			}
			data, readErr := io.ReadAll(img)
			if readErr != nil {
				return nil
			}
			imgByPage[pageNr] = append(imgByPage[pageNr], data)
			return nil
		}, conf)
		pdfFile.Close()
	}

	results := make([]model.PageContent, 0, endPage-startPage+1)
	for pageNum := startPage; pageNum <= endPage; pageNum++ {
		content := model.PageContent{PageNumber: pageNum}
		if text, e := extractPageTextFromReader(r, pageNum); e == nil {
			content.Text = text
			content.HasMeaningfulText = len(strings.TrimSpace(text)) >= meaningfulTextThreshold
		}
		content.Images = imgByPage[pageNum]
		results = append(results, content)
	}
	return results, nil
}

// ExtractPageContent extracts text and embedded images from a single page.
func ExtractPageContent(pdfPath string, pageNum int) (model.PageContent, error) {
	pages, err := ExtractPagesRange(pdfPath, pageNum, pageNum)
	if err != nil || len(pages) == 0 {
		return model.PageContent{PageNumber: pageNum}, err
	}
	return pages[0], nil
}

// SplitPDF extracts pages [startPage, endPage] inclusive into outputPath.
func SplitPDF(pdfPath string, startPage, endPage int, outputPath string) error {
	pageRange := fmt.Sprintf("%d-%d", startPage, endPage)
	conf := pdfcpumodel.NewDefaultConfiguration()
	return pdfcpuapi.CollectFile(pdfPath, outputPath, []string{pageRange}, conf)
}

// PageCache memoizes page extraction across concurrent callers for a single PDF.
type PageCache struct {
	mu      sync.Map // key: int (pageNum) → model.PageContent
	pdfPath string
	fetchMu sync.Mutex // serializes fetches to avoid duplicate range extraction
}

// NewPageCache creates a cache for the given PDF file.
func NewPageCache(pdfPath string) *PageCache {
	return &PageCache{pdfPath: pdfPath}
}

// GetRange returns pages [start, end] inclusive, fetching any uncached pages.
func (c *PageCache) GetRange(start, end int) ([]model.PageContent, error) {
	// Find the contiguous span of uncached pages
	fetchStart, fetchEnd := -1, -1
	for p := start; p <= end; p++ {
		if _, ok := c.mu.Load(p); !ok {
			if fetchStart == -1 {
				fetchStart = p
			}
			fetchEnd = p
		}
	}

	if fetchStart != -1 {
		c.fetchMu.Lock()
		// Re-check after acquiring the lock (another goroutine may have fetched)
		needsFetch := false
		for p := fetchStart; p <= fetchEnd; p++ {
			if _, ok := c.mu.Load(p); !ok {
				needsFetch = true
				break
			}
		}
		if needsFetch {
			pages, err := ExtractPagesRange(c.pdfPath, fetchStart, fetchEnd)
			if err != nil {
				c.fetchMu.Unlock()
				return nil, err
			}
			for _, pg := range pages {
				c.mu.Store(pg.PageNumber, pg)
			}
		}
		c.fetchMu.Unlock()
	}

	results := make([]model.PageContent, end-start+1)
	for i, p := range makeRange(start, end) {
		if v, ok := c.mu.Load(p); ok {
			results[i] = v.(model.PageContent)
		} else {
			results[i] = model.PageContent{PageNumber: p}
		}
	}
	return results, nil
}

// extractPageTextFromReader extracts text from an already-opened ledpdf reader.
func extractPageTextFromReader(r *ledpdf.Reader, pageNum int) (string, error) {
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

func makeRange(start, end int) []int {
	s := make([]int, end-start+1)
	for i := range s {
		s[i] = start + i
	}
	return s
}
