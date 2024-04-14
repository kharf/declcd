package build

#regexManager: {
	customType: "regex"
	fileMatch: [...string]
	matchStrings: [...string]
	matchStringsStrategy?: string
	depNameTemplate?:      string
	versioningTemplate?:   string
	datasourceTemplate?:   string
	registryUrlTemplate?:  string
	packageNameTemplate?:  string
}

#cueModuleManager: #regexManager & {
	fileMatch: [
		"^(.*?)cue.mod/module.cue$",
	]
	versioningTemplate: "semver-coerced"
}

_cueModuleVersionManager: #cueModuleManager & {
	matchStrings: ["version: \"(?<currentValue>.*?)\""]
	depNameTemplate:    "cue-lang/cue"
	datasourceTemplate: "github-releases"
}

_githubReleaseManager: #regexManager & {
	fileMatch: [
		"^(.*?).go$",
		"^(.*?).cue$",
	]
	matchStrings: ["https://github.com/(?<depName>.*?)/releases/download/(?<currentValue>.*?)/"]
	datasourceTemplate: "github-releases"
	versioningTemplate: "semver-coerced"
}

labels: [
	"dependencies",
	"renovate",
]
extends: [
	"config:best-practices",
]
dependencyDashboard:   true
semanticCommits:       "enabled"
rebaseWhen:            "auto"
branchConcurrentLimit: 0
prConcurrentLimit:     0
prHourlyLimit:         0
"github-actions": enabled: false
postUpdateOptions: [
	"gomodTidy",
]
customManagers: [
	_cueModuleVersionManager,
	{
		customType: "regex"
		fileMatch: [
			"^(.*?).cue$",
		]
		matchStrings: ["uses: \"(?<depName>.*?)@(?<currentValue>.*?)\""]
		datasourceTemplate: "github-tags"
		versioningTemplate: "semver-coerced"
	}, {
		customType: "regex"
		fileMatch: [
			"^(.*?).go$",
		]
		matchStrings: ["From\\(\"(?<depName>.*?):(?<currentValue>.*?)\"\\)"]
		datasourceTemplate: "docker"
	},
	{
		customType: "regex"
		fileMatch: [
			"^(.*?).go$",
		]
		matchStrings: [
			"var cueDep = \"(?<depName>.*?)@(?<currentValue>.*?)\"",
			"var controllerGenDep = \"(?<depName>.*?)@(?<currentValue>.*?)\"",
		]
		datasourceTemplate: "go"
	},
	_githubReleaseManager,
]
