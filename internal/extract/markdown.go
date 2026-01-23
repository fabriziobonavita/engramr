package extract

import (
	"strings"
	"unicode"
)

// Chunk is a markdown-derived chunk ready for embedding/storage.
type Chunk struct {
	SourcePath  string
	HeadingPath []string
	ChunkIndex  int
	Content     string
}

type section struct {
	headingPath []string
	content     string
}

// ChunkMarkdown splits markdown content into chunks, preserving heading context.
func ChunkMarkdown(sourcePath string, markdown string) []Chunk {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return nil
	}

	sections := splitSections(markdown)

	// Used for "skip tiny chunk unless file is tiny".
	fileLen := len(markdown)

	var out []Chunk
	for _, s := range sections {
		cs := chunkSection(sourcePath, s, fileLen)
		out = append(out, cs...)
	}

	return out
}

func splitSections(markdown string) []section {
	lines := strings.Split(markdown, "\n")

	var (
		curHeadingPath []string
		curLines       []string
		sections       []section
	)

	flush := func() {
		content := strings.TrimSpace(strings.Join(curLines, "\n"))
		if content == "" {
			curLines = nil
			return
		}
		sections = append(sections, section{
			headingPath: append([]string(nil), curHeadingPath...),
			content:     content,
		})
		curLines = nil
	}

	for _, line := range lines {
		if level, title, ok := parseHeading(line); ok {
			flush()
			// update heading path
			if level <= 0 {
				continue
			}
			if level-1 < len(curHeadingPath) {
				curHeadingPath = curHeadingPath[:level-1]
			}
			// If headings jump levels (level-1 > len(curHeadingPath)), just treat as next level
			// under current without inserting empties - we simply append the title.
			curHeadingPath = append(curHeadingPath, title)
			continue
		}

		curLines = append(curLines, line)
	}

	flush()
	return sections
}

func parseHeading(line string) (level int, title string, ok bool) {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}

	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return 0, "", false
	}
	if i < len(trimmed) && trimmed[i] != ' ' && trimmed[i] != '\t' {
		return 0, "", false
	}

	title = strings.TrimSpace(trimmed[i:])
	if title == "" {
		return 0, "", false
	}
	return i, title, true
}

func chunkSection(sourcePath string, s section, fileLen int) []Chunk {
	const (
		minTarget = 1500
		maxSize   = 2500
		overlap   = 200
	)

	sec := strings.TrimSpace(s.content)
	if sec == "" {
		return nil
	}

	// Section small enough: single chunk.
	if len(sec) <= maxSize {
		// This noise filtering is not useful for now.
		//if len(sec) < 200 && fileLen >= 200 {
		//	return nil
		//}
		return []Chunk{{
			SourcePath:  sourcePath,
			HeadingPath: append([]string(nil), s.headingPath...),
			ChunkIndex:  0,
			Content:     sec,
		}}
	}

	paras := splitParagraphs(sec)
	var chunks []string

	var buf strings.Builder
	bufLen := 0
	flush := func() {
		c := strings.TrimSpace(buf.String())
		buf.Reset()
		bufLen = 0
		if c != "" {
			chunks = append(chunks, c)
		}
	}

	for _, p := range paras {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		sep := ""
		if bufLen > 0 {
			sep = "\n\n"
		}
		addLen := len(sep) + len(p)

		// If adding this paragraph would exceed max, flush current buffer.
		if bufLen > 0 && bufLen+addLen > maxSize {
			flush()
			sep = ""
			addLen = len(p)
		}

		// If paragraph itself is huge, hard-split it.
		if addLen > maxSize {
			// flush current first
			if bufLen > 0 {
				flush()
			}
			for len(p) > 0 {
				n := maxSize
				if len(p) < n {
					n = len(p)
				}
				part := strings.TrimSpace(p[:n])
				if part != "" {
					chunks = append(chunks, part)
				}
				p = p[n:]
			}
			continue
		}

		buf.WriteString(sep)
		buf.WriteString(p)
		bufLen += addLen

		// If we've reached minimum target and next paragraph would overflow, we flush later.
		if bufLen >= minTarget && bufLen >= maxSize {
			flush()
		}
	}
	flush()

	// Apply overlap between adjacent chunks (same section).
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1]
		tail := prev
		if len(tail) > overlap {
			tail = tail[len(tail)-overlap:]
		}
		cur := tail + "\n" + chunks[i]
		if len(cur) > maxSize {
			cur = cur[:maxSize]
		}
		chunks[i] = strings.TrimSpace(cur)
	}

	var out []Chunk
	for i, c := range chunks {
		if len(c) < 200 && fileLen >= 200 {
			continue
		}
		out = append(out, Chunk{
			SourcePath:  sourcePath,
			HeadingPath: append([]string(nil), s.headingPath...),
			ChunkIndex:  i,
			Content:     strings.TrimSpace(c),
		})
	}

	return out
}

func splitParagraphs(section string) []string {
	// Paragraphs separated by one or more blank lines.
	lines := strings.Split(section, "\n")
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		paras = append(paras, strings.Join(cur, "\n"))
		cur = nil
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return paras
}
