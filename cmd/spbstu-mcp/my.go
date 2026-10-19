package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type myIn struct {
	Semester   int    `json:"semester,omitempty" jsonschema:"semester number, e.g. 7; omit for all"`
	RecordBook string `json:"record_book,omitempty" jsonschema:"record book id from my_record_books; defaults to the first"`
}

type noIn struct{}

func registerMy(s *mcp.Server, my myService) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "my_record_books", Annotations: readOnly,
		Description: "The student's record books (зачётные книжки) on my.spbstu.ru, one per programme, with the group."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, any, error) {
			v, err := my.RecordBooks(ctx)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "my_grades", Annotations: readOnly,
		Description: "Grades from the record book on my.spbstu.ru (зачётка): discipline, control type, grade, date, " +
			"lecturer, and the average grade of every semester."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in myIn) (*mcp.CallToolResult, any, error) {
			v, err := my.Grades(ctx, in.RecordBook, in.Semester)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "my_study_plan", Annotations: readOnly,
		Description: "The programme's study plan (учебный план) on my.spbstu.ru: disciplines with semesters, hours and control type."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in myIn) (*mcp.CallToolResult, any, error) {
			v, err := my.StudyPlan(ctx, in.RecordBook, in.Semester)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "my_work_plan", Annotations: readOnly,
		Description: "This year's work plan (рабочий план) on my.spbstu.ru by semester: disciplines, control type, " +
			"lecture, practice, lab and self-study hours."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in myIn) (*mcp.CallToolResult, any, error) {
			v, err := my.WorkPlan(ctx, in.RecordBook, in.Semester)
			if err != nil {
				return nil, nil, err
			}
			return result(v)
		})
}
