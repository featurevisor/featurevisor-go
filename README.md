# Featurevisor Go SDK <!-- omit in toc -->

This is a port of Featurevisor [Javascript SDK](https://featurevisor.com/docs/sdks/javascript/) v3.x to Go, providing a way to evaluate feature flags, variations, and variables in your Go applications.

This SDK is compatible with [Featurevisor](https://featurevisor.com/) v3.0 projects and v2 datafiles.

See example application [here](https://github.com/featurevisor/featurevisor-example-go).

## Table of contents <!-- omit in toc -->

- [Installation](#installation)
- [Public API](#public-api)
- [Initialization](#initialization)
- [Evaluation types](#evaluation-types)
- [Context](#context)
  - [Setting initial context](#setting-initial-context)
  - [Setting after initialization](#setting-after-initialization)
  - [Replacing existing context](#replacing-existing-context)
  - [Manually passing context](#manually-passing-context)
- [Check if enabled](#check-if-enabled)
- [Getting variation](#getting-variation)
- [Getting variables](#getting-variables)
  - [Type specific methods](#type-specific-methods)
- [Getting all evaluations](#getting-all-evaluations)
- [Sticky](#sticky)
  - [Initialize with sticky](#initialize-with-sticky)
  - [Set sticky afterwards](#set-sticky-afterwards)
- [Setting datafile](#setting-datafile)
  - [Merging by default](#merging-by-default)
  - [Replacing](#replacing)
  - [Loading datafiles on demand](#loading-datafiles-on-demand)
  - [Updating datafile](#updating-datafile)
  - [Interval-based update](#interval-based-update)
- [Diagnostics](#diagnostics)
  - [Levels](#levels)
  - [Handler](#handler)
- [Events](#events)
  - [`datafile_set`](#datafile_set)
  - [`context_set`](#context_set)
  - [`sticky_set`](#sticky_set)
  - [`error`](#error)
- [Evaluation details](#evaluation-details)
- [Modules](#modules)
  - [Defining a module](#defining-a-module)
  - [Registering modules](#registering-modules)
- [Child instance](#child-instance)
- [Close](#close)
- [CLI usage](#cli-usage)
  - [Test](#test)
  - [Benchmark](#benchmark)
  - [Assess distribution](#assess-distribution)
- [Development of this package](#development-of-this-package)
  - [Setting up](#setting-up)
  - [Running tests](#running-tests)
  - [Releasing](#releasing)
- [License](#license)

<!-- FEATUREVISOR_DOCS_BEGIN -->

## Installation

In your Go application, install the SDK using Go modules:

```bash
go get github.com/featurevisor/featurevisor-go
```

## Public API

The main runtime API is `featurevisor.CreateFeaturevisor()`:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Datafile: datafileContent,
})
```

Most applications only need `CreateFeaturevisor`, the `Featurevisor` instance type, and `FeaturevisorOptions`. Public extension and observability types include `FeaturevisorModule`, `FeaturevisorDiagnostic`, and the datafile model types.

## Initialization

The SDK can be initialized by passing [datafile](https://featurevisor.com/docs/building-datafiles/) content directly:

```go
package main

import (
    "io"
    "net/http"

    "github.com/featurevisor/featurevisor-go"
)

func main() {
    datafileURL := "https://cdn.yoursite.com/datafile.json"

    resp, err := http.Get(datafileURL)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    datafileBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        panic(err)
    }

    var datafileContent featurevisor.DatafileContent
    if err := datafileContent.FromJSON(string(datafileBytes)); err != nil {
        panic(err)
    }

    f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
        Datafile: datafileContent,
    })
}
```

## Evaluation types

We can evaluate 3 types of values against a particular [feature](https://featurevisor.com/docs/features/):

- [**Flag**](#check-if-enabled) (`bool`): whether the feature is enabled or not
- [**Variation**](#getting-variation) (`string`): the variation of the feature (if any)
- [**Variables**](#getting-variables): variable values of the feature (if any)

These evaluations are run against the provided context.

## Context

Contexts are [attribute](https://featurevisor.com/docs/attributes) values that we pass to SDK for evaluating [features](https://featurevisor.com/docs/features) against.

Think of the conditions that you define in your [segments](https://featurevisor.com/docs/segments/), which are used in your feature's [rules](https://featurevisor.com/docs/features/#rules).

They are plain maps:

```go
context := featurevisor.Context{
    "userId": "123",
    "country": "nl",
    // ...other attributes
}
```

Context can be passed to SDK instance in various different ways, depending on your needs:

### Setting initial context

You can set context at the time of initialization:

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Context: featurevisor.Context{
        "deviceId": "123",
        "country":  "nl",
    },
})
```

This is useful for values that don't change too frequently and available at the time of application startup.

### Setting after initialization

You can also set more context after the SDK has been initialized:

```go
f.SetContext(featurevisor.Context{
    "userId": "234",
})
```

This will merge the new context with the existing one (if already set).

### Replacing existing context

If you wish to fully replace the existing context, you can pass `true` in second argument:

```go
f.SetContext(featurevisor.Context{
    "deviceId": "123",
    "userId":   "234",
    "country":  "nl",
    "browser":  "chrome",
}, true) // replace existing context
```

### Manually passing context

You can optionally pass additional context manually for each and every evaluation separately, without needing to set it to the SDK instance affecting all evaluations:

```go
context := featurevisor.Context{
    "userId": "123",
    "country": "nl",
}

isEnabled := f.IsEnabled("my_feature", context)
variation := f.GetVariation("my_feature", context)
variableValue := f.GetVariable("my_feature", "my_variable", context)
```

When manually passing context, it will merge with existing context set to the SDK instance before evaluating the specific value.

Further details for each evaluation types are described below.

## Check if enabled

Once the SDK is initialized, you can check if a feature is enabled or not:

```go
featureKey := "my_feature"

isEnabled := f.IsEnabled(featureKey)

if isEnabled {
    // do something
}
```

You can also pass additional context per evaluation:

```go
isEnabled := f.IsEnabled(featureKey, featurevisor.Context{
    // ...additional context
})
```

## Getting variation

If your feature has any [variations](https://featurevisor.com/docs/features/#variations) defined, you can evaluate them as follows:

```go
featureKey := "my_feature"

variation := f.GetVariation(featureKey)

if variation != nil && *variation == "treatment" {
    // do something for treatment variation
} else {
    // handle default/control variation
}
```

Additional context per evaluation can also be passed:

```go
variation := f.GetVariation(featureKey, featurevisor.Context{
    // ...additional context
})
```

## Getting variables

Your features may also include [variables](https://featurevisor.com/docs/features/#variables), which can be evaluated as follows:

```go
variableKey := "bgColor"

bgColorValue := f.GetVariable("my_feature", variableKey)
```

Additional context per evaluation can also be passed:

```go
bgColorValue := f.GetVariable("my_feature", variableKey, featurevisor.Context{
    // ...additional context
})
```

### Type specific methods

Next to generic `GetVariable()` methods, there are also type specific methods available for convenience:

```go
f.GetVariableBoolean(featureKey, variableKey, context)
f.GetVariableString(featureKey, variableKey, context)
f.GetVariableInteger(featureKey, variableKey, context)
f.GetVariableDouble(featureKey, variableKey, context)
f.GetVariableArray(featureKey, variableKey, context)
f.GetVariableObject(featureKey, variableKey, context)
f.GetVariableJSON(featureKey, variableKey, context)
```

Type specific methods do not coerce values. `GetVariableInteger()` returns `nil` for the string `"1"`, and `GetVariableBoolean()` returns `nil` for the string `"true"`.

For typed arrays/objects, use `Into` methods with pointer outputs:

```go
var items []string
_ = f.GetVariableArrayInto(featureKey, variableKey, context, &items)

var cfg MyConfig
_ = f.GetVariableObjectInto(featureKey, variableKey, context, &cfg)
```

`context` and `OverrideOptions` are optional and can be passed before the output pointer.

## Getting all evaluations

You can get evaluations of all features available in the SDK instance:

```go
allEvaluations := f.GetAllEvaluations(featurevisor.Context{})

fmt.Printf("%+v\n", allEvaluations)
// {
//   myFeature: {
//     enabled: true,
//     variation: "control",
//     variables: {
//       myVariableKey: "myVariableValue",
//     },
//   },
//
//   anotherFeature: {
//     enabled: true,
//     variation: "treatment",
//   }
// }
```

This is handy especially when you want to pass all evaluations from a backend application to the frontend.

## Sticky

For the lifecycle of the SDK instance in your application, you can set some features with sticky values, meaning that they will not be evaluated against the fetched [datafile](https://featurevisor.com/docs/building-datafiles/):

Sticky values belong to an SDK or child instance. Evaluation options do not accept sticky overrides; create a child with `SpawnOptions{Sticky: ...}` when a child needs its own sticky state.

### Initialize with sticky

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Sticky: &featurevisor.StickyFeatures{
        "myFeatureKey": {
            Enabled: true,
            // optional
            Variation: func() *featurevisor.VariationValue {
                v := featurevisor.VariationValue("treatment")
                return &v
            }(),
            Variables: map[string]interface{}{
                "myVariableKey": "myVariableValue",
            },
        },
        "anotherFeatureKey": {
            Enabled: false,
        },
    },
})
```

Once initialized with sticky features, the SDK will look for values there first before evaluating the targeting conditions and going through the bucketing process.

### Set sticky afterwards

You can also set sticky features after the SDK is initialized:

```go
f.SetSticky(featurevisor.StickyFeatures{
    "myFeatureKey": {
        Enabled: true,
        Variation: func() *featurevisor.VariationValue {
            v := featurevisor.VariationValue("treatment")
            return &v
        }(),
        Variables: map[string]interface{}{
            "myVariableKey": "myVariableValue",
        },
    },
    "anotherFeatureKey": {
        Enabled: false,
    },
}, true) // replace existing sticky features (false by default)
```

## Setting datafile

You may also initialize the SDK without passing `datafile`, and set it later on:

```go
f.SetDatafile(datafileContent)
```

`SetDatafile` accepts either parsed `featurevisor.DatafileContent` or a raw JSON string.

### Merging by default

By default, `SetDatafile(datafile)` merges the incoming datafile with the SDK instance's existing datafile:

- incoming `Features` and `Segments` override matching keys
- existing `Features` and `Segments` that are missing from the incoming datafile are kept
- `Revision`, `SchemaVersion`, and `FeaturevisorVersion` are taken from the incoming datafile

This means you can call `SetDatafile` more than once with different datafiles, and the SDK instance accumulates their features and segments together.

### Replacing

Pass `true` as the second argument to replace the stored datafile entirely:

```go
f.SetDatafile(datafileContent, true) // replace existing datafile
```

### Loading datafiles on demand

Because merging is the default, a single SDK instance can start with a small datafile and load more datafiles later as your application needs them, instead of downloading every feature upfront.

This pairs well with [targets](https://featurevisor.com/docs/targets/), where each target produces a smaller datafile for a specific part of your application:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{})

func loadDatafile(target string) {
    url := fmt.Sprintf("https://cdn.yoursite.com/production/featurevisor-%s.json", target)
    resp, err := http.Get(url)
    if err != nil {
        return
    }
    defer resp.Body.Close()

    datafileBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return
    }

    f.SetDatafile(string(datafileBytes))
}

loadDatafile("products")

// later, when the user reaches checkout
loadDatafile("checkout")
```

### Updating datafile

You can set the datafile as many times as you want in your application, which will result in emitting a [`datafile_set`](#datafile-set) event that you can listen and react to accordingly.

The triggers for setting the datafile again can be:

- periodic updates based on an interval (like every 5 minutes), or
- reacting to:
  - a specific event in your application (like a user action), or
  - an event served via websocket or server-sent events (SSE)

### Interval-based update

Here's an example of using interval-based update:

```go
import (
    "time"
    "io"
    "net/http"

    "github.com/featurevisor/featurevisor-go"
)

func updateDatafile(f *featurevisor.Featurevisor, datafileURL string) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        resp, err := http.Get(datafileURL)
        if err != nil {
            continue
        }
        defer resp.Body.Close()

        datafileBytes, err := io.ReadAll(resp.Body)
        if err != nil {
            continue
        }

        var datafileContent featurevisor.DatafileContent
        if err := datafileContent.FromJSON(string(datafileBytes)); err != nil {
            continue
        }

        f.SetDatafile(datafileContent)
    }
}

// Start the update goroutine
go updateDatafile(f, datafileURL)
```

## Diagnostics

By default, Featurevisor reports diagnostics to the console for `info` level and above with a `[Featurevisor]` prefix.

### Levels

Available diagnostic levels are `fatal`, `error`, `warn`, `info`, and `debug`.

Set the level during initialization or update it afterwards:

```go
logLevel := featurevisor.LogLevelDebug
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    LogLevel: &logLevel,
})

f.SetLogLevel(featurevisor.LogLevelInfo)
```

### Handler

Use `OnDiagnostic` to send structured diagnostics to your observability system:

```go
f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    LogLevel: &logLevel,
    OnDiagnostic: func(diagnostic featurevisor.FeaturevisorDiagnostic) {
        fmt.Println(diagnostic.Level, diagnostic.Code, diagnostic.Message)
    },
})
```

Modules can also subscribe to diagnostics or report their own from `Setup` via the provided module API.

Every diagnostic has `Level`, `Code`, `Message`, and an object-shaped `Details` map. Optional `Module`, `ModuleName`, and `OriginalError` fields describe provenance. Evaluation metadata belongs in `Details`.

Diagnostic handlers are isolated from SDK behavior. A panic in a handler does not stop other handlers or evaluations.


## Events

Featurevisor SDK implements a simple event emitter that allows you to listen to events that happen in the runtime.

You can listen to these events that can occur at various stages in your application:

### `datafile_set`

```go
unsubscribe := f.On(featurevisor.EventNameDatafileSet, func(details featurevisor.EventDetails) {
    revision := details["revision"]               // new revision
    previousRevision := details["previousRevision"]
    revisionChanged := details["revisionChanged"] // true if revision has changed

    // list of feature keys that have new updates,
    // and you should re-evaluate them
    features := details["features"]

    // handle here
})

// stop listening to the event
unsubscribe()
```

The `features` array will contain keys of features that have either been:

- added, or
- updated, or
- removed

compared to the previous datafile content that existed in the SDK instance.

### `context_set`

```go
unsubscribe := f.On(featurevisor.EventNameContextSet, func(details featurevisor.EventDetails) {
    replaced := details["replaced"] // true if context was replaced
    context := details["context"]   // the new context

    fmt.Println("Context set")
})
```

### `sticky_set`

```go
unsubscribe := f.On(featurevisor.EventNameStickySet, func(details featurevisor.EventDetails) {
    replaced := details["replaced"] // true if sticky features got replaced
    features := details["features"] // list of all affected feature keys

    fmt.Println("Sticky features set")
})
```

### `error`

```go
unsubscribe := f.On(featurevisor.EventNameError, func(details featurevisor.EventDetails) {
    diagnostic := details["diagnostic"]
    fmt.Println(diagnostic)
})
```

## Evaluation details

Besides logging with debug level enabled, you can also get more details about how the feature variations and variables are evaluated in the runtime against given context:

```go
// flag
evaluation := f.EvaluateFlag(featureKey, context)

// variation
evaluation := f.EvaluateVariation(featureKey, context)

// variable
evaluation := f.EvaluateVariable(featureKey, variableKey, context)
```

The returned object will always contain the following properties:

- `FeatureKey`: the feature key
- `Reason`: the reason how the value was evaluated

And optionally these properties depending on whether you are evaluating a feature variation or a variable:

- `BucketValue`: the bucket value between 0 and 100,000
- `RuleKey`: the rule key
- `Error`: the error object
- `Enabled`: if feature itself is enabled or not
- `Variation`: the variation object
- `VariationValue`: the variation value
- `VariableKey`: the variable key
- `VariableValue`: the variable value
- `VariableSchema`: the variable schema
- `VariableOverrideIndex`: index of matched variable override when applicable

## Modules

Modules allow you to intercept the evaluation process and customize it further as per your needs.

### Defining a module

A module is a simple struct with a recommended unique `Name` and optional functions:

If `Setup` panics, the module is not registered. Featurevisor removes subscriptions created during setup, reports `module_setup_error`, and calls `Close` when present.

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

myCustomModule := &featurevisor.FeaturevisorModule{
    // recommended for diagnostics and removal
    Name: "my-custom-module",

    // rest of the properties below are all optional per module
    Setup: func(api featurevisor.FeaturevisorModuleApi) {
        api.ReportDiagnostic(featurevisor.FeaturevisorModuleReportedDiagnostic{
            Level:   featurevisor.LogLevelInfo,
            Code:    "module_ready",
            Message: "Module is ready",
        })
    },

    // before evaluation
    Before: func(options featurevisor.EvaluateOptions) featurevisor.EvaluateOptions {
        // update context before evaluation
        if options.Context == nil {
            options.Context = featurevisor.Context{}
        }
        options.Context["someAdditionalAttribute"] = "value"
        return options
    },

    // after evaluation
    After: func(evaluation featurevisor.Evaluation, options featurevisor.EvaluateOptions) featurevisor.Evaluation {
        if evaluation.Reason == "error" {
            // log error
            return evaluation
        }
        return evaluation
    },

    // configure bucket key
    BucketKey: func(options featurevisor.ConfigureBucketKeyOptions) featurevisor.BucketKey {
        // return custom bucket key
        return options.BucketKey
    },

    // configure bucket value (between 0 and 100,000)
    BucketValue: func(options featurevisor.ConfigureBucketValueOptions) featurevisor.BucketValue {
        // return custom bucket value
        return options.BucketValue
    },

    Close: func() {
        // clean up module resources
    },
}
```

### Registering modules

You can register modules at the time of SDK initialization:

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

f := featurevisor.CreateFeaturevisor(featurevisor.FeaturevisorOptions{
    Modules: []*featurevisor.FeaturevisorModule{
        myCustomModule,
    },
})
```

Or after initialization:

```go
removeModule := f.AddModule(myCustomModule)
removeModule()
```

## Child instance

When dealing with purely client-side applications, it is understandable that there is only one user involved, like in browser or mobile applications.

But when using Featurevisor SDK in server-side applications, where a single server instance can handle multiple user requests simultaneously, it is important to isolate the context for each request.

That's where child instances come in handy:

```go
childF := f.Spawn(featurevisor.Context{
    // user or request specific context
    "userId": "123",
})
```

Now you can pass the child instance where your individual request is being handled, and you can continue to evaluate features targeting that specific user alone:

```go
isEnabled := childF.IsEnabled("my_feature")
variation := childF.GetVariation("my_feature")
variableValue := childF.GetVariable("my_feature", "my_variable")
```

Similar to parent SDK, child instances also support several additional methods:

- `SetContext`
- `SetSticky`
- `IsEnabled`
- `GetVariation`
- `GetVariable`
- `GetVariableBoolean`
- `GetVariableString`
- `GetVariableInteger`
- `GetVariableDouble`
- `GetVariableArray`
- `GetVariableArrayInto`
- `GetVariableObject`
- `GetVariableObjectInto`
- `GetVariableJSON`
- `GetAllEvaluations`
- `On`
- `Close`

## Close

Both primary and child instances support a `.Close()` method, that removes forgotten event listeners (via `On` method) and cleans up any potential memory leaks.

```go
f.Close()
```

## CLI usage

This package also provides a CLI tool for running your Featurevisor [project](https://featurevisor.com/docs/projects/)'s test specs and benchmarking against this Go SDK:

All three commands accept repeatable `--target=<target>` options. `test` builds only the selected Target datafiles and runs untargeted assertions plus assertions for those targets. `benchmark` and `assess-distribution` run independently against every selected Target datafile. Without `--target`, existing project-wide behavior is preserved. Project definitions, test specs, Target discovery, and datafile generation continue to come from the Node.js CLI.

### Test

Learn more about testing [here](https://featurevisor.com/docs/testing/).

```bash
go run cmd/main.go test --projectDirectoryPath="/absolute/path/to/your/featurevisor/project"
```

Additional options that are available:

```bash
go run cmd/main.go test \
    --projectDirectoryPath="/absolute/path/to/your/featurevisor/project" \
    --quiet|verbose \
    --onlyFailures \
    --keyPattern="myFeatureKey" \
    --assertionPattern="#1"
```

If you want to validate parity locally against the JavaScript SDK runner, you can use the bundled example project:

```bash
cd /Users/fahad/Projects/featurevisor/featurevisor/examples/example-1
npx featurevisor test

# from this Go SDK repository root:
go run cmd/main.go test \
  --projectDirectoryPath="/Users/fahad/Projects/featurevisor/featurevisor/examples/example-1" \
  --onlyFailures

# or:
make test-example-1
```

### Benchmark

Learn more about benchmarking [here](https://featurevisor.com/docs/cli/#benchmarking).

```bash
go run cmd/main.go benchmark \
    --projectDirectoryPath="/absolute/path/to/your/featurevisor/project" \
    --environment="production" \
    --feature="myFeatureKey" \
    --context='{"country": "nl"}' \
    --n=1000
```

### Assess distribution

Learn more about assessing distribution [here](https://featurevisor.com/docs/cli/#assess-distribution).

```bash
go run cmd/main.go assess-distribution \
    --projectDirectoryPath="/absolute/path/to/your/featurevisor/project" \
    --environment=production \
    --feature=foo \
    --variation \
    --context='{"country": "nl"}' \
    --populateUuid=userId \
    --populateUuid=deviceId \
    --n=1000
```

<!-- FEATUREVISOR_DOCS_END -->

## Development of this package

### Setting up

Clone the repository, and install the dependencies using Go modules:

```bash
go mod download
```

### Running tests

```bash
go test ./...
```

### Releasing

- Manually create a new release on [GitHub](https://github.com/featurevisor/featurevisor-go/releases)
- Tag it with a prefix of `v`, like `v1.0.0`

## License

MIT © [Fahad Heylaal](https://fahad19.com)
