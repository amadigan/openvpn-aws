package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Parser struct {
	reader      *bufio.Reader
	sectionType string
	sectionName string
	line        int
}

type ConfigStatement struct {
	SectionType string
	SectionName string
	Word        string
	Fields      []string
	Line        int
}

func Open(data io.Reader) *Parser {
	return &Parser{reader: bufio.NewReader(data)}
}

func (p *Parser) Read() (*ConfigStatement, error) {
	for {
		line, err := p.reader.ReadString('\n')
		p.line++

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			return nil, err
		}

		line = strings.TrimSuffix(line, "\n")

		ind := strings.IndexRune(line, '#')

		if ind != -1 {
			line = line[:ind]
		}

		fields := strings.Fields(line)

		if len(fields) == 0 {
			continue
		}

		if line[0] == ' ' || line[0] == '\t' {
			if p.sectionType == "" {
				return nil, fmt.Errorf("config:%d illegal line %s outside section", p.line, line)
			}

			return &ConfigStatement{
				SectionType: p.sectionType,
				SectionName: p.sectionName,
				Word:        fields[0],
				Fields:      fields[1:],
				Line:        p.line,
			}, nil
		} else {
			stmt := new(ConfigStatement)
			stmt.Line = p.line
			if fields[0] == "global" {
				stmt.SectionType = "global"
				stmt.SectionName = ""
			} else if len(fields) == 2 {
				stmt.SectionType = fields[0]
				stmt.SectionName = fields[1]
			} else {
				return nil, fmt.Errorf("config:%d illegal section declaration %s", p.line, line)
			}

			p.sectionType = stmt.SectionType
			p.sectionName = stmt.SectionName

			return stmt, nil
		}

	}

	return nil, nil
}
