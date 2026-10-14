package workflow

import (
	"github.com/superdurable/xtemporal/common/dynamicconfig"
	"github.com/superdurable/xtemporal/common/retrypolicy"
)

type Config struct {
	maxIDLengthLimit                  dynamicconfig.IntPropertyFn
	defaultWorkflowRetrySettings      dynamicconfig.TypedPropertyFnWithNamespaceFilter[retrypolicy.DefaultRetrySettings]
	maxLinksPerRequest                dynamicconfig.IntPropertyFnWithNamespaceFilter
	linkMaxSize                       dynamicconfig.IntPropertyFnWithNamespaceFilter
	enableSignalWithStartFromWorkflow dynamicconfig.BoolPropertyFnWithNamespaceFilter
}

func NewConfig(dc *dynamicconfig.Collection) Config {
	return Config{
		maxIDLengthLimit:                  dynamicconfig.MaxIDLengthLimit.Get(dc),
		defaultWorkflowRetrySettings:      dynamicconfig.DefaultWorkflowRetryPolicy.Get(dc),
		maxLinksPerRequest:                dynamicconfig.FrontendMaxLinksPerRequest.Get(dc),
		linkMaxSize:                       dynamicconfig.FrontendLinkMaxSize.Get(dc),
		enableSignalWithStartFromWorkflow: dynamicconfig.EnableSignalWithStartFromWorkflow.Get(dc),
	}
}
