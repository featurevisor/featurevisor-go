# Featurevisor Go SDK <!-- omit in toc -->

This is a port of Featurevisor [Javascript SDK](https://featurevisor.com/docs/sdks/javascript/) v2.x to Go, providing a way to evaluate feature flags, variations, and variables in your Go applications.

This SDK is compatible with [Featurevisor](https://featurevisor.com/) v2.0 projects and above.

See example application [here](https://github.com/featurevisor/featurevisor-example-go).

## Table of contents <!-- omit in toc -->

- [Installation](#installation)
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
  - [Updating datafile](#updating-datafile)
  - [Interval-based update](#interval-based-update)
- [Logging](#logging)
  - [Levels](#levels)
  - [Customizing levels](#customizing-levels)
  - [Handler](#handler)
- [Events](#events)
  - [`datafile_set`](#datafile_set)
  - [`context_set`](#context_set)
  - [`sticky_set`](#sticky_set)
- [Evaluation details](#evaluation-details)
- [Hooks](#hooks)
  - [Defining a hook](#defining-a-hook)
  - [Registering hooks](#registering-hooks)
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

    f := featurevisor.CreateInstance(featurevisor.Options{
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

f := featurevisor.CreateInstance(featurevisor.Options{
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

### Initialize with sticky

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

f := featurevisor.CreateInstance(featurevisor.Options{
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

## Logging

By default, Featurevisor SDKs will print out logs to the console for `info` level and above.

### Levels

These are all the available log levels:

- `error`
- `warn`
- `info`
- `debug`

### Customizing levels

If you choose `debug` level to make the logs more verbose, you can set it at the time of SDK initialization.

Setting `debug` level will print out all logs, including `info`, `warn`, and `error` levels.

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

logLevel := featurevisor.LogLevelDebug
f := featurevisor.CreateInstance(featurevisor.Options{
    LogLevel: &logLevel,
})
```

Alternatively, you can also set `logLevel` directly:

```go
logLevel := featurevisor.LogLevelDebug
f := featurevisor.CreateInstance(featurevisor.Options{
    LogLevel: &logLevel,
})
```

You can also set log level from SDK instance afterwards:

```go
f.SetLogLevel(featurevisor.LogLevelDebug)
```

### Handler

You can also pass your own log handler, if you do not wish to print the logs to the console:

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

logger := featurevisor.NewLogger(featurevisor.CreateLoggerOptions{
    Level: &featurevisor.LogLevelInfo,
    Handler: func(level featurevisor.LogLevel, message string, details interface{}) {
        // do something with the log
    },
})

f := featurevisor.CreateInstance(featurevisor.Options{
    Logger: logger,
})
```

Further log levels like `info` and `debug` will help you understand how the feature variations and variables are evaluated in the runtime against given context.

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

## Hooks

Hooks allow you to intercept the evaluation process and customize it further as per your needs.

### Defining a hook

A hook is a simple struct with a unique required `Name` and optional functions:

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

myCustomHook := &featurevisor.Hook{
    // only required property
    Name: "my-custom-hook",

    // rest of the properties below are all optional per hook

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
    After: func(evaluation featurevisor.Evaluation, options featurevisor.EvaluateOptions) {
        if evaluation.Reason == "error" {
            // log error
            return
        }
    },

    // configure bucket key
    BucketKey: func(options featurevisor.EvaluateOptions) string {
        // return custom bucket key
        return options.BucketKey
    },

    // configure bucket value (between 0 and 100,000)
    BucketValue: func(options featurevisor.EvaluateOptions) int {
        // return custom bucket value
        return options.BucketValue
    },
}
```

### Registering hooks

You can register hooks at the time of SDK initialization:

```go
import (
    "github.com/featurevisor/featurevisor-go"
)

f := featurevisor.CreateInstance(featurevisor.Options{
    Hooks: []*featurevisor.Hook{
        myCustomHook,
    },
})
```

Or after initialization:

```go
removeHook := f.AddHook(myCustomHook)
removeHook()
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
    --with-scopes \
    --with-tags \
    --keyPattern="myFeatureKey" \
    --assertionPattern="#1"
```

`--with-scopes` and `--with-tags` match Featurevisor CLI behavior by generating and testing against scoped/tagged datafiles via `npx featurevisor build`.

If you want to validate parity locally against the JavaScript SDK runner, you can use the bundled example project:

```bash
cd monorepo/examples/example-1
npx featurevisor test --with-scopes --with-tags

# from repository root:
go run cmd/main.go test \
  --projectDirectoryPath="/absolute/path/to/featurevisor-go/monorepo/examples/example-1" \
  --with-scopes \
  --with-tags
```

### Benchmark

Learn more about benchmarking [here](https://featurevisor.com/docs/cmd/#benchmarking).

```bash
go run cmd/main.go benchmark \
    --projectDirectoryPath="/absolute/path/to/your/featurevisor/project" \
    --environment="production" \
    --feature="myFeatureKey" \
    --context='{"country": "nl"}' \
    --n=1000
```

### Assess distribution

Learn more about assessing distribution [here](https://featurevisor.com/docs/cmd/#assess-distribution).

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
