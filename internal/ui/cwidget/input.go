package cwidget

import (
	"errors"
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Input[T any] struct {
	widget.BaseWidget

	labelWidget *widget.Label
	entryWidget *widget.Entry
	errorWidget *widget.Label

	LabelText   string
	Placeholder string

	DefaultValue T

	OnChanged   func(T)
	OnSubmitted func(T)

	Validator func(string) (T, error)
}

func NewIntInput(label, placeholder string, defaultValue int, onChanged func(int)) *Input[int] {
	input := &Input[int]{
		LabelText:    label,
		Placeholder:  placeholder,
		OnChanged:    onChanged,
		DefaultValue: defaultValue,
	}

	input.labelWidget = widget.NewLabel(fmt.Sprintf("%s: %d", label, input.DefaultValue))
	input.labelWidget.TextStyle = fyne.TextStyle{Bold: true}

	input.entryWidget = widget.NewEntry()
	input.entryWidget.SetPlaceHolder(placeholder)

	input.errorWidget = widget.NewLabel("")
	input.errorWidget.Hidden = true
	input.errorWidget.TextStyle = fyne.TextStyle{Italic: true}
	input.errorWidget.Importance = widget.DangerImportance

	input.Validator = func(s string) (res int, err error) {
		if s == "" {
			return input.DefaultValue, nil
		}

		res, err = strconv.Atoi(s)

		if res == 0 {
			return input.DefaultValue, errors.New("zero error")
		}

		return
	}

	input.entryWidget.OnChanged = func(s string) {
		res, err := input.Validator(s)
		input.SetError(err)

		if err == nil {
			input.OnChanged(res)
			input.labelWidget.SetText(fmt.Sprintf("%s: %d", label, res))
		}
	}

	input.ExtendBaseWidget(input)

	return input
}

func (item *Input[T]) CreateRenderer() fyne.WidgetRenderer {
	c := container.NewVBox(
		item.labelWidget,
		item.entryWidget,
		item.errorWidget,
	)

	return widget.NewSimpleRenderer(c)
}

func (item *Input[T]) SetError(err error) {
	item.errorWidget.Hidden = err == nil
	if err != nil {
		item.errorWidget.SetText(err.Error())
	}
}

func (item *Input[T]) SetText(text string) {
	item.entryWidget.SetText(text)
}
