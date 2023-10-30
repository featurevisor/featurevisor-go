# featurevisor-go

Go SDK for [Featurevisor](https://featurevisor.com).

Under heavy development. Not ready yet.

## TODOs

- [ ] Types
  - [x] Initial batch of types
  - [ ] Create parser functions for `DatafileContent` and `Test`
- [x] Bucketing
  - [x] Add bucketing functions following murmurhashv3 algorith
  - [x] Add tests using same fixtures as JS SDK
- [x] Logger
  - [x] Create `Logger` struct
  - [x] Add tests
- [ ] DatafileReader
  - [ ] Create `DatafileReader` struct
  - [ ] Add tests
- [ ] Conditions
  - [ ] Write conditions evaluator
  - [ ] Add tests
- [ ] Segments
  - [ ] Write segments evaluator
  - [ ] Add tests
- [ ] Emitter
  - [ ] Create Emitter class, keeping multithreading in mind
  - [ ] Add tests
- [ ] Feature
  - [ ] Create common functions for feature evaluation
- [ ] Instance
  - [ ] Create `Instance` struct
  - [ ] Options:
    - [ ] `bucketKeySeparator`
    - [ ] `configureBucketKey`
    - [ ] `configureBucketValue`
    - [ ] `datafile`
    - [ ] `datafileUrl`
    - [ ] `handleDatafileFetch`
    - [ ] `initialFeatures`
    - [ ] `interceptContext`
    - [ ] `logger`
    - [ ] `onActivation`
    - [ ] `onReady`
    - [ ] `onRefresh`
    - [ ] `onUpdate`
    - [ ] `refreshInterval`
    - [ ] `stickyFeatures`
  - [ ] Methods:
    - [ ] `onReady`
    - [ ] `setDatafile`
    - [ ] `setStickyFeatures`
    - [ ] `getRevision`
    - [ ] `getFeature`
    - [ ] `getBucketKey`
    - [ ] `getBucketValue`
    - [ ] `isReady`
    - [ ] `refresh`
    - [ ] `startRefreshing`
    - [ ] `stopRefreshing`
    - [ ] `evaluateFlag`
    - [ ] `isEnabled`
    - [ ] `evaluateVariation`
    - [ ] `getVariation`
    - [ ] `activate`
    - [ ] `evaluateVariable`
    - [ ] `getVariable`
    - [ ] `getVariableBoolean`
    - [ ] `getVariableString`
    - [ ] `getVariableInteger`
    - [ ] `getVariableDouble`
    - [ ] `getVariableArray`
    - [ ] `getVariableObject`
    - [ ] `getVariableJSON`
  - [ ] `createInstance` function
  - [ ] Add tests
- [ ] Test runner
  - [ ] Create an executable `featurevisor-go` that runs tests

## Local development

```
$ go build
$ go test
```

## License

[MIT](./LICENSE)
