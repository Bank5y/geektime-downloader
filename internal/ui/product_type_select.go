package ui

import (
	"github.com/manifoldco/promptui"
)

type ProductTypeSelectOption struct {
	Index              int
	Text               string
	SourceType         int
	AcceptProductTypes []string
	NeedSelectArticle  bool
	IsEnterpriseMode   bool
}

// ProductTypeOptions returns all selectable product types
func ProductTypeOptions(isEnterprise bool) []ProductTypeSelectOption {
	if isEnterprise {
		return []ProductTypeSelectOption{
			{0, "训练营", 5, []string{"c44"}, true, true}, //custom source type, not use
		}
	}
	return []ProductTypeSelectOption{
		{0, "普通课程", 1, []string{"c1", "c3"}, true, false},
		{1, "每日一课", 2, []string{"d"}, false, false},
		{2, "公开课", 1, []string{"p35", "p29", "p30"}, true, false},
		{3, "大厂案例", 4, []string{"q"}, false, false},
		{4, "训练营", 5, []string{""}, true, false}, //custom source type, not use
		{5, "其他", 1, []string{"x", "c6"}, true, false},
	}
}

// ProductTypeOptionByCode finds the non-enterprise product type that accepts the given product type code
func ProductTypeOptionByCode(code string) (ProductTypeSelectOption, bool) {
	if code == "" {
		return ProductTypeSelectOption{}, false
	}
	for _, o := range ProductTypeOptions(false) {
		for _, c := range o.AcceptProductTypes {
			if c == code {
				return o, true
			}
		}
	}
	return ProductTypeSelectOption{}, false
}

// Accepts reports whether product type code is accepted by this option
func (p *ProductTypeSelectOption) Accepts(code string) bool {
	for _, c := range p.AcceptProductTypes {
		if c == code {
			return true
		}
	}
	return false
}

func ProductTypeSelect(isEnterprise bool) (ProductTypeSelectOption, error) {
	productTypeOptions := ProductTypeOptions(isEnterprise)

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "{{ `>` | red }} {{ .Text | red }}",
		Inactive: "{{ .Text }}",
	}
	prompt := promptui.Select{
		Label:        "请选择想要下载的产品类型",
		Items:        productTypeOptions,
		Templates:    templates,
		Size:         len(productTypeOptions),
		HideSelected: true,
		Stdout:       NoBellStdout,
	}
	index, _, err := prompt.Run()
	if err != nil {
		return ProductTypeSelectOption{}, err
	}
	return productTypeOptions[index], nil
}

// IsUniversity checks if the product type is university product type
func (p *ProductTypeSelectOption) IsUniversity() bool {
	return p.Index == 4 && !p.IsEnterpriseMode
}
