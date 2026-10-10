package config

import "testing"

func TestFlowDefiniteVariablesAndIndexScope(t *testing.T) {
	request := Step{Name: "Use", URL: "/{{token|path}}"}
	producer := func(name string) Step {
		return Step{Name: name, URL: "/", Extract: map[string]Extraction{"token": {Header: "X-Token"}}}
	}
	tests := []struct {
		name  string
		steps []Step
		valid bool
	}{
		{"both paths define", []Step{{Name: "Choose", If: &Branch{Condition: Condition{Variable: "user_id", Value: ptr("1")}, Then: []Step{producer("A")}, Else: []Step{producer("B")}}}, request}, true},
		{"one path defines", []Step{{If: &Branch{Condition: Condition{Variable: "user_id", Value: ptr("1")}, Then: []Step{producer("A")}}}, request}, false},
		{"exists guard", []Step{{If: &Branch{Condition: Condition{Variable: "token", Operator: "exists"}, Then: []Step{request}}}}, true},
		{"not exists false guard", []Step{{If: &Branch{Condition: Condition{Variable: "token", Operator: "not_exists"}, Else: []Step{request}}}}, true},
		{"fixed loop defines", []Step{{Loop: &Loop{Count: ptr(2), Steps: []Step{producer("A")}}}, request}, true},
		{"while may not run", []Step{{Loop: &Loop{While: &Condition{Variable: "user_id", Value: ptr("1")}, MaxIterations: 2, Steps: []Step{producer("A")}}}, request}, false},
		{"index in body", []Step{{Loop: &Loop{Count: ptr(2), IndexVariable: "item", Steps: []Step{{URL: "/{{item}}"}}}}}, true},
		{"index cannot escape", []Step{{Loop: &Loop{Count: ptr(2), IndexVariable: "item", Steps: []Step{{URL: "/{{item}}"}}}}, {URL: "/{{item}}"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := scenarioConfig()
			c.Scenarios[0].Steps = tt.steps
			_, err := c.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}

func TestInvalidFlowsAreRejected(t *testing.T) {
	body := []Step{{Name: "Request", URL: "/"}}
	tests := []Step{
		{URL: "/", If: &Branch{Then: body}},
		{If: &Branch{Then: body}, Loop: &Loop{Count: ptr(1), Steps: body}},
		{Loop: &Loop{Count: ptr(0), Steps: body}},
		{Loop: &Loop{Count: ptr(1001), Steps: body}},
		{Loop: &Loop{Count: ptr(1), MaxIterations: 2, Steps: body}},
		{Loop: &Loop{While: &Condition{Variable: "user_id", Value: ptr("1")}, Steps: body}},
		{Loop: &Loop{Count: ptr(1), IndexVariable: "journey_id", Steps: body}},
		{If: &Branch{Condition: Condition{Variable: "missing", Value: ptr("1")}, Then: body}},
		{If: &Branch{Condition: Condition{Variable: "user_id", Operator: "eval", Value: ptr("true")}, Then: body}},
		{If: &Branch{Condition: Condition{Variable: "user_id", Operator: "exists", Value: ptr("")}, Then: body}},
		{If: &Branch{Condition: Condition{Variable: "user_id"}, Then: body}},
		{If: &Branch{Condition: Condition{Variable: "user_id", Value: ptr("1")}}},
		{Loop: &Loop{Count: ptr(100), Steps: []Step{{Loop: &Loop{Count: ptr(100), Steps: body}}}}},
	}
	for i, step := range tests {
		c := scenarioConfig()
		c.Scenarios[0].Steps = []Step{step}
		if _, err := c.Validate(); err == nil {
			t.Errorf("invalid flow %d accepted", i)
		}
	}
	nested := Step{URL: "/"}
	for i := 0; i < 10; i++ {
		nested = Step{Loop: &Loop{Count: ptr(1), Steps: []Step{nested}}}
	}
	c := scenarioConfig()
	c.Scenarios[0].Steps = []Step{nested}
	if _, err := c.Validate(); err == nil {
		t.Fatal("deep nesting accepted")
	}
}

func TestConditions(t *testing.T) {
	for _, tt := range []struct {
		op, actual, expected string
		want                 bool
	}{
		{"equals", "", "", true}, {"not_equals", "a", "b", true}, {"contains", "abc", "b", true}, {"not_contains", "abc", "d", true},
		{"gt", "2", "1", true}, {"gte", "2", "2", true}, {"lt", "-2", "1", true}, {"lte", "2", "2", true}, {"gt", "1", "2", false},
	} {
		c, err := compileCondition(Condition{Variable: "x", Operator: tt.op, Value: &tt.expected}, map[string]bool{"x": true})
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.Evaluate(map[string]string{"x": tt.actual})
		if err != nil || got != tt.want {
			t.Fatalf("%+v got %v %v", tt, got, err)
		}
	}
	c, _ := compileCondition(Condition{Variable: "x", Operator: "gt", Value: ptr("1")}, map[string]bool{"x": true})
	for _, value := range []string{"NaN", "Inf", "not a number"} {
		if _, err := c.Evaluate(map[string]string{"x": value}); err == nil {
			t.Errorf("invalid number %q", value)
		}
	}
	if _, err := c.Evaluate(nil); err == nil {
		t.Fatal("missing comparison value accepted")
	}
}
