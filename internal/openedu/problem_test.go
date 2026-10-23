package openedu

import (
	"html"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const problemContent = `<h3 class="hd hd-3 problem-header">Вопрос 1</h3>
<div class="problem"><p>Дано отношение:</p><table><caption>R</caption>
<tr><td><p>A</p></td><td><p>B</p></td></tr>
<tr><td>1</td><td>2</td></tr></table>
<div class="choicegroup capa_inputtype"><fieldset><legend>Сколько в нём кортежей?</legend>
<div class="field"><input type="radio" name="input_x_2_1" id="input_x_2_1_choice_0" value="choice_0"/><label for="input_x_2_1_choice_0" class="response-label"> Один </label></div>
<div class="field"><input type="radio" name="input_x_2_1" id="input_x_2_1_choice_1" value="choice_1" checked="true"/><label for="input_x_2_1_choice_1" class="response-label"> Два </label></div>
</fieldset><div class="indicator-container"><span class="status incorrect"><span class="sr">неверно</span></span></div></div>
<div class="choicegroup capa_inputtype"><fieldset><legend>Отметьте ключи</legend>
<input type="checkbox" name="input_x_3_1[]" id="c0" value="choice_0"/><label for="c0">A</label>
<input type="checkbox" name="input_x_3_1[]" id="c1" value="choice_1"/><label for="c1">B</label>
<input type="checkbox" name="input_x_3_1[]" id="c2" value="choice_2"/><label for="c2"><img src="/asset-v1:x+type@asset+block@er.png"/></label>
</fieldset><span class="status unanswered"></span></div>
<p>Подъязык команды <img src="/asset-v1:x+type@asset+block@drop.png"/></p>
<select name="input_x_4_1"><option value="option_x_4_1_dummy_default">Выберите опцию</option><option value="DDL" selected="true"> DDL</option><option value="DML">DML</option></select>
<label>Название операции</label><input type="text" name="input_x_5_1" value=""/>
</div><div class="action"><button>Отправить</button><div class="submission-feedback">Вы использовали 1 из 2 попыток<span class="sr">подсказка</span></div></div>`

func TestParseProblems(t *testing.T) {
	page := `<div class="xblock" data-block-type="problem" data-graded="True"><div class="problems-wrapper"
 data-problem-id="block-v1:c+type@problem+block@x" data-url="/courses/c/xblock/x/handler/xmodule_handler"
 data-problem-score="0" data-problem-total-possible="4" data-content="` + html.EscapeString(problemContent) + `"></div></div>`
	ps := parseProblems([]byte(page))
	if len(ps) != 1 {
		t.Fatalf("got %d problems", len(ps))
	}
	p := ps[0]
	if p.Title != "Вопрос 1" || p.Score != "0/4" || p.Attempts != "1/2" || !p.Graded || p.url != LMSURL+"/courses/c/xblock/x/handler/xmodule_handler" {
		t.Errorf("problem: %+v", p)
	}
	want := []Input{
		{Name: "input_x_2_1", Type: "radio", Prompt: "Дано отношение:\nR | A | B |\n| 1 | 2 |\nСколько в нём кортежей?",
			Choices: []Choice{{"choice_0", "Один"}, {"choice_1", "Два"}}, Answer: []string{"choice_1"}, Status: "incorrect"},
		{Name: "input_x_3_1", Type: "checkbox", Prompt: "Отметьте ключи", Choices: []Choice{{"choice_0", "A"}, {"choice_1", "B"}, {"choice_2", "[изображение: " + LMSURL + "/asset-v1:x+type@asset+block@er.png]"}}, Status: "unanswered"},
		{Name: "input_x_4_1", Type: "select", Prompt: "Подъязык команды [изображение: " + LMSURL + "/asset-v1:x+type@asset+block@drop.png]",
			Choices: []Choice{{"DDL", "DDL"}, {"DML", "DML"}}, Answer: []string{"DDL"}},
		{Name: "input_x_5_1", Type: "text", Prompt: "Название операции"},
	}
	if !reflect.DeepEqual(p.Inputs, want) {
		t.Errorf("inputs:\n%+v\nwant\n%+v", p.Inputs, want)
	}

	if !needsReset(p.Inputs) || needsReset(p.Inputs[1:]) {
		t.Error("needsReset: want true only with an incorrect input")
	}

	form, err := answerForm(p.Inputs, map[string][]string{"input_x_3_1": {"choice_0", "choice_1"}, "input_x_5_1": {"объединение"}})
	if err != nil {
		t.Fatal(err)
	}
	wantForm := url.Values{"input_x_2_1": {"choice_1"}, "input_x_3_1[]": {"choice_0", "choice_1"}, "input_x_4_1": {"DDL"}, "input_x_5_1": {"объединение"}}
	if !reflect.DeepEqual(form, wantForm) {
		t.Errorf("form %v, want %v", form, wantForm)
	}
	for _, bad := range []map[string][]string{{"input_x_9_1": {"a"}}, {"input_x_2_1": {"choice_7"}}, {"input_x_2_1": {"choice_0", "choice_1"}}} {
		if _, err := answerForm(p.Inputs, bad); err == nil || !strings.Contains(err.Error(), "input_x_") {
			t.Errorf("%v: err %v", bad, err)
		}
	}
}

func TestQuestionsSuffix(t *testing.T) {
	for in, want := range map[string]string{
		"Промежуточный экзамен (20 Questions)": "Промежуточный экзамен",
		"Неделя 1. Тест (1 Question)":          "Неделя 1. Тест",
		"Итоговый экзамен":                     "Итоговый экзамен",
	} {
		if got := questionsRe.ReplaceAllString(in, ""); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// A text input inside a sentence: the words after it belong to the question too.
func TestParseTrailingText(t *testing.T) {
	content := `<h3 class="hd hd-3 problem-header">Вопрос по теме 2</h3>
<div class="problem"><div><div class="inline" role="group"><p style="display: inline;">Тег с именем <div class="capa_inputtype inline textline"><div class="unanswered inline">
<input type="text" name="input_y_2_1" value=""/><span class="trailing_text"></span>
<span class="status unanswered"><span class="sr">нет ответа</span></span><p class="answer"></p></div></div><p style="display: inline;">добавляет картинки на веб-страницу.</p></p></div></div></div>
<div class="action"><input type="hidden" name="problem_id" value="Вопрос по теме 2"/><button class="submit"><span>Отправить</span></button>
<div class="submission-feedback">Вы использовали 0 из 3 попыток<span class="sr">подсказка</span></div></div>`
	page := `<div class="problems-wrapper" data-problem-id="p" data-content="` + html.EscapeString(content) + `"></div>`
	ps := parseProblems([]byte(page))
	if len(ps) != 1 || len(ps[0].Inputs) != 1 {
		t.Fatalf("got %+v", ps)
	}
	if got, want := ps[0].Inputs[0].Prompt, "Тег с именем ___ добавляет картинки на веб-страницу."; got != want {
		t.Errorf("prompt %q, want %q", got, want)
	}
	if ps[0].Attempts != "0/3" {
		t.Errorf("attempts %q", ps[0].Attempts)
	}
}
