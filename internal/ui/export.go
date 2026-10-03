package ui

import (
	"context"
	"github.com/ealink1/super-link/internal/datafile"
	"github.com/ealink1/super-link/internal/domain"
	"io"
)

func (s *workspace) exportResults() {
	if len(s.lastResults) == 0 {
		s.status.SetText("没有可以导出的结果")
		return
	}
	index := s.results.SelectedIndex()
	if s.logTab != nil {
		index -= 2
	}
	s.exportResult(max(0, index))
}

func (s *workspace) exportResult(index int) {
	if index < 0 || index >= len(s.lastResults) {
		return
	}
	request := s.lastRequest
	if len(s.lastResults) != 1 || request.Write || len(request.Parameters) != 0 {
		request.Text = ""
	}
	s.owner.exportDialog(exportSource{profile: s.lastProfile, result: s.lastResults[index], request: request})
}
func writeResult(ctx context.Context, writer io.Writer, kind string, result domain.Result) error {
	encoder, err := datafile.NewEncoder(ctx, writer, datafile.Options{Format: kind})
	if err != nil {
		return err
	}
	defer encoder.Close()
	columns := []string{}
	for _, c := range result.Columns {
		columns = append(columns, c.Name)
	}
	if err = encoder.SetColumns(columns); err != nil {
		return err
	}
	for _, row := range result.Rows {
		if err = encoder.ConsumeRowValues(row); err != nil {
			return err
		}
	}
	return encoder.Finish()
}
func csvText(value string) string { return datafile.CSVText(value) }
