package chromedp

import (
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"gonum.org/v1/gonum/stat/combin"
	"strconv"
	"strings"
	"time"
)

const (
	lessonUrlTmpl        = "https://lis.itmo.ru/1/lesson/%s"
	lisUrl               = "https://lis.itmo.ru/1/map"
	submitButtonSelector = `button[class="button button_medium button_primary task-basis__submit"]`
)

var opts = append(chromedp.DefaultExecAllocatorOptions[:],
	chromedp.Flag("headless", false),
	chromedp.ProxyServer("http://127.0.0.1:8080"),
)

func ProcessLesson(ctx context.Context) {
	allocatorCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	chromedpCtx, cancel1 := chromedp.NewContext(allocatorCtx)
	defer cancel1()

	err := chromedp.Run(chromedpCtx, chromedp.Navigate(lisUrl))
	if err != nil {
		panic(err)
	}

	for {
		processForm(chromedpCtx)
		var html string
		chromedp.Run(chromedpCtx,
			chromedp.OuterHTML("html", &html),
		)

		reader := strings.NewReader(html)

		doc, _ := goquery.NewDocumentFromReader(reader)
		if doc.Find(`button[class="replica-variant__button"]`).Length() == 0 {
			continue
		}
		chromedp.Run(
			chromedpCtx,
			chromedp.WaitVisible(`button[class="replica-variant__button"]`, chromedp.ByQuery),
			chromedp.Click(`button[class="replica-variant__button"]`, chromedp.ByQuery),
		)
		time.Sleep(1 * time.Second)
	}
}

func processForm(ctx context.Context) {
	var html string
	chromedp.Run(ctx,
		chromedp.OuterHTML("html", &html),
	)

	reader := strings.NewReader(html)

	doc, _ := goquery.NewDocumentFromReader(reader)

	forms := doc.Find("div[class='task-basis']")
	if forms.Length() == 0 {
		fmt.Println("Forms not found")
		return
	}

	form := forms.Last()

	submitButton := form.Find(submitButtonSelector)
	if submitButton.Length() == 0 {
		fmt.Println("Submit button not found")
		return
	}

	formId, ok := submitButton.Attr("form")
	if !ok {
		fmt.Println("Form id not found")
		return
	}

	form = doc.Find(fmt.Sprintf(`form[id="%s"]`, formId))
	if form.Length() == 0 {
		fmt.Println("Form not found")
		return
	}

	formType, ok := form.Attr("class")
	if !ok {
		fmt.Println("Form type not found")
		return
	}

	switch formType {
	case "t2-checkboxes":
		processCheckboxes(ctx, formId, form)
	case "t1-radios":
		processRadioButtons(ctx, formId, form)
	case "t6-table":
		processTable(ctx, formId, form)
	}
}

func processCheckboxes(ctx context.Context, formId string, form *goquery.Selection) {

	checkboxesAmount := form.Find(`div[class="t2-option"]`).Length()

	if checkboxesAmount == 0 {
		fmt.Println("Checkboxes not found")
		return
	}

	for i := 1; i <= checkboxesAmount; i++ {
		combinations := combin.Combinations(checkboxesAmount, i)
		for _, combination := range combinations {
			variants := make([]string, 0)
			for _, index := range combination {
				variants = append(variants, fmt.Sprintf("%s_var%d", formId, index))
			}
			if submitCheckboxes(ctx, formId, variants) {
				return
			}
		}
	}
}

func submitCheckboxes(ctx context.Context, formId string, variants []string) bool {
	for _, variant := range variants {
		chromedp.Run(ctx,
			chromedp.Click(fmt.Sprintf(`label[for="%s"]`, variant), chromedp.ByQuery),
			chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
		)
		//time.Sleep(100 * time.Millisecond)
	}

	return submitForm(ctx, formId)
}

func processRadioButtons(ctx context.Context, formId string, form *goquery.Selection) {
	radioButtonsAmount := form.Find(`div[class="t1-option"]`).Length()

	if radioButtonsAmount == 0 {
		fmt.Println("Radio buttons not found")
		return
	}

	for i := 0; i < radioButtonsAmount; i++ {
		chromedp.Run(ctx,
			chromedp.Click(fmt.Sprintf(`label[for="%s_var%d"]`, formId, i), chromedp.ByQuery),
			chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
		)
		//time.Sleep(100 * time.Millisecond)
		if submitForm(ctx, formId) {
			return
		}
	}

}

func processTable(ctx context.Context, formId string, form *goquery.Selection) {
	rows := form.Find(`div[class="t6-table__row-header"]`).Length()
	if rows == 0 {
		fmt.Println("Rows not found")
		return
	}

	cols := form.Find(`div[class="t6-table__column-header"]`).Length()
	if cols == 0 {
		fmt.Println("Columns not found")
		return
	}

	btnTmpl := "%s_row-%d__%s_col-"
	variants := make(map[string]int)
	for i := 0; i < rows; i++ {
		variants[fmt.Sprintf(btnTmpl, formId, i, formId)] = 0
	}

	for k, v := range variants {
		chromedp.Run(ctx,
			chromedp.Click(fmt.Sprintf(`label[for="%s"]`, k+strconv.Itoa(v)), chromedp.ByQuery),
			chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
		)
		//time.Sleep(100 * time.Millisecond)
	}

	stop := false

	opt := func(doc *goquery.Document) {
		for k, v := range variants {
			label := doc.Find(fmt.Sprintf(`label[for="%s"]`, k+strconv.Itoa(v)))
			if label.Length() < 0 {
				fmt.Println("Label not found")
				stop = true
				return
			}
			class, ok := label.Attr("class")
			if !ok {
				fmt.Println("Label class not found")
				stop = true
				return
			}

			if strings.Contains(class, "box_correct") {
				continue
			}

			if v == cols-1 {
				stop = true
				return
			}
			variants[k]++
		}
	}

	correct := submitForm(ctx, formId, opt)

	for !correct && !stop {
		for k, v := range variants {
			chromedp.Run(ctx,
				chromedp.Click(fmt.Sprintf(`label[for="%s"]`, k+strconv.Itoa(v)), chromedp.ByQuery),
				chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
			)
			//time.Sleep(100 * time.Millisecond)
		}

		correct = submitForm(ctx, formId, opt)
	}

}

func submitForm(ctx context.Context, formId string, opts ...func(doc *goquery.Document)) bool {
	submitSel := fmt.Sprintf(`button[form="%s"]`, formId)
	chromedp.Run(ctx,
		chromedp.Click(submitSel, chromedp.ByQuery),
	)

	time.Sleep(1 * time.Second)

	var html string
	chromedp.Run(ctx,
		chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
		chromedp.OuterHTML("html", &html),
	)

	reader := strings.NewReader(html)

	doc, _ := goquery.NewDocumentFromReader(reader)

	for _, opt := range opts {
		opt(doc)
	}

	if doc.Find(submitSel).Length() > 0 {
		chromedp.Run(ctx,
			chromedp.Click(submitSel, chromedp.ByQuery),
			chromedp.WaitReady(fmt.Sprintf(`form[id="%s"]`, formId)),
		)
		//time.Sleep(100 * time.Millisecond)
		return false
	}

	return true
}
