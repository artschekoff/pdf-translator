package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/pdf-translator/pdf-translator/internal/docling"
	"github.com/pdf-translator/pdf-translator/internal/domain"
	"go.uber.org/zap"
)

// MarkdownTranslator translates markdown sections in batch.
type MarkdownTranslator interface {
	TranslateMarkdownSections(ctx context.Context, sections []string, sourceLang, targetLang string, batchSize int) ([]string, error)
}

// MarkdownRenderer renders a translated markdown string to a PDF file.
type MarkdownRenderer interface {
	RenderMarkdown(ctx context.Context, markdown, targetLang, outputPath string) error
}

// DoclingPipeline runs the full docling extraction → translation → rendering flow.
type DoclingPipeline struct {
	client     *docling.Client
	translator MarkdownTranslator
	renderer   MarkdownRenderer
	logger     *zap.Logger
}

func NewDoclingPipeline(client *docling.Client, trans MarkdownTranslator, rend MarkdownRenderer, logger *zap.Logger) *DoclingPipeline {
	return &DoclingPipeline{
		client:     client,
		translator: trans,
		renderer:   rend,
		logger:     logger,
	}
}

// Run executes the full docling pipeline for the given translation request.
func (p *DoclingPipeline) Run(ctx context.Context, req *domain.TranslateRequest) error {
	p.logger.Info("starting docling pipeline",
		zap.String("input", req.InputPath),
		zap.String("output", req.OutputPath),
		zap.String("targetLang", req.TargetLang),
		zap.Bool("keepOriginal", req.KeepOriginal),
	)

	p.logger.Info("converting PDF with docling")
	result, err := p.client.Convert(ctx, req.InputPath, docling.ConvertRequest{
		DoOcr:            true,
		DoTableStructure: true,
		ImagesScale:      2.0,
	})
	if err != nil {
		return fmt.Errorf("docling convert: %w", err)
	}
	p.logger.Info("docling conversion done", zap.Int("pages", result.Pages))

	markdown, images := extractMDImages(result.Markdown)
	sections := splitMDSections(markdown)
	p.logger.Info("split markdown", zap.Int("sections", len(sections)))

	p.logger.Info("translating sections", zap.String("targetLang", req.TargetLang))
	translated, err := p.translator.TranslateMarkdownSections(ctx, sections, req.SourceLang, req.TargetLang, 0)
	if err != nil {
		return fmt.Errorf("translating markdown: %w", err)
	}

	var outputMD string
	if req.KeepOriginal {
		pairs := make([]string, len(sections))
		for i := range sections {
			trans := ""
			if i < len(translated) {
				trans = translated[i]
			}
			pairs[i] = sections[i] + "\n\n" + trans
		}
		outputMD = strings.Join(pairs, "\n\n---\n\n")
	} else {
		outputMD = strings.Join(translated, "\n\n")
	}
	outputMD = restoreMDImages(outputMD, images)

	p.logger.Info("rendering markdown to PDF", zap.String("output", req.OutputPath))
	if err := p.renderer.RenderMarkdown(ctx, outputMD, req.TargetLang, req.OutputPath); err != nil {
		return fmt.Errorf("rendering PDF: %w", err)
	}

	p.logger.Info("docling pipeline complete", zap.String("output", req.OutputPath))
	return nil
}

var mdImgRE = regexp.MustCompile(`<!--\s*IMG_\d+\s*-->|!\[[^\]]*\]\([^)]{0,8192}\)`)

func extractMDImages(md string) (string, map[string]string) {
	images := make(map[string]string)
	counter := 0
	result := mdImgRE.ReplaceAllStringFunc(md, func(m string) string {
		key := fmt.Sprintf("<!-- IMG_%d -->", counter)
		images[key] = m
		counter++
		return key
	})
	return result, images
}

func restoreMDImages(md string, images map[string]string) string {
	for key, orig := range images {
		md = strings.ReplaceAll(md, key, orig)
	}
	return md
}

var mdHeadingRE = regexp.MustCompile(`^#{1,6}\s+`)

func splitMDSections(md string) []string {
	var sections []string
	var current strings.Builder

	scanner := bufio.NewScanner(strings.NewReader(md))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if mdHeadingRE.MatchString(line) && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		sections = append(sections, s)
	}
	if len(sections) == 0 {
		return []string{md}
	}
	return sections
}
