package commands

import (
	"encoding/json"
	"fmt"

	"github.com/featurevisor/featurevisor-go/sdk"
	"github.com/featurevisor/featurevisor-go/types"
)

func Evaluate(args []string) {
	parsedArgs := ParseArgs(args)

	// Assume datafile content exists as a string in a variable
	datafileContent := `
{
  "schemaVersion": "1",
  "revision": "3",
  "attributes": [
    {
      "key": "country",
      "type": "string"
    },
    {
      "key": "date",
      "type": "string"
    },
    {
      "key": "device",
      "type": "string"
    },
    {
      "key": "userId",
      "type": "string",
      "capture": true
    },
    {
      "key": "version",
      "type": "string"
    }
  ],
  "segments": [
    {
      "key": "blackFridayWeekend",
      "conditions": "{\"and\":[{\"attribute\":\"date\",\"operator\":\"after\",\"value\":\"2023-11-24T00:00:00.000Z\"},{\"attribute\":\"date\",\"operator\":\"before\",\"value\":\"2023-11-27T00:00:00.000Z\"}]}"
    },
    {
      "key": "desktop",
      "conditions": "[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"desktop\"}]"
    },
    {
      "key": "germany",
      "conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"de\"}]}"
    },
    {
      "key": "mobile",
      "conditions": "{\"and\":[{\"attribute\":\"device\",\"operator\":\"equals\",\"value\":\"mobile\"}]}"
    },
    {
      "key": "netherlands",
      "conditions": "[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"nl\"}]"
    },
    {
      "key": "switzerland",
      "conditions": "{\"and\":[{\"attribute\":\"country\",\"operator\":\"equals\",\"value\":\"ch\"}]}"
    },
    {
      "key": "version_gt5",
      "conditions": "[{\"attribute\":\"version\",\"operator\":\"semverGreaterThan\",\"value\":\"5.0.0\"}]"
    }
  ],
  "features": [
    {
      "key": "bar",
      "bucketBy": "userId",
      "variations": [
        {
          "value": "control",
          "weight": 33
        },
        {
          "value": "b",
          "weight": 33,
          "variables": [
            {
              "key": "hero",
              "value": {
                "title": "Hero Title for B",
                "subtitle": "Hero Subtitle for B",
                "alignment": "center for B"
              },
              "overrides": [
                {
                  "segments": "{\"or\":[\"germany\",\"switzerland\"]}",
                  "value": {
                    "title": "Hero Title for B in DE or CH",
                    "subtitle": "Hero Subtitle for B in DE of CH",
                    "alignment": "center for B in DE or CH"
                  }
                }
              ]
            }
          ]
        },
        {
          "value": "c",
          "weight": 34
        }
      ],
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 50000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                0,
                16500
              ]
            },
            {
              "variation": "b",
              "range": [
                16500,
                33000
              ]
            },
            {
              "variation": "c",
              "range": [
                33000,
                50000
              ]
            }
          ]
        }
      ],
      "ranges": [
        [
          0,
          50000
        ]
      ],
      "variablesSchema": [
        {
          "key": "color",
          "type": "string",
          "defaultValue": "red"
        },
        {
          "key": "hero",
          "type": "object",
          "defaultValue": {
            "title": "Hero Title",
            "subtitle": "Hero Subtitle",
            "alignment": "center"
          }
        }
      ]
    },
    {
      "key": "baz",
      "bucketBy": {
        "or": [
          "userId",
          "device"
        ]
      },
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 80000,
          "allocation": []
        }
      ]
    },
    {
      "key": "checkout",
      "bucketBy": "userId",
      "traffic": [
        {
          "key": "1",
          "segments": "netherlands",
          "percentage": 100000,
          "allocation": [],
          "variables": {
            "paymentMethods": [
              "ideal",
              "paypal"
            ]
          }
        },
        {
          "key": "2",
          "segments": "germany",
          "percentage": 100000,
          "allocation": [],
          "variables": {
            "paymentMethods": [
              "sofort",
              "paypal"
            ]
          }
        },
        {
          "key": "3",
          "segments": "*",
          "percentage": 100000,
          "allocation": [],
          "variables": {
            "showPayments": true,
            "showShipping": true,
            "paymentMethods": [
              "visa",
              "mastercard",
              "paypal"
            ]
          }
        }
      ],
      "variablesSchema": [
        {
          "key": "showPayments",
          "type": "boolean",
          "defaultValue": false
        },
        {
          "key": "showShipping",
          "type": "boolean",
          "defaultValue": false
        },
        {
          "key": "paymentMethods",
          "type": "array",
          "defaultValue": [
            "visa",
            "mastercard"
          ]
        }
      ]
    },
    {
      "key": "discount",
      "bucketBy": "userId",
      "required": [
        "sidebar"
      ],
      "traffic": [
        {
          "key": "2",
          "segments": "[\"blackFridayWeekend\"]",
          "percentage": 100000,
          "allocation": []
        },
        {
          "key": "1",
          "segments": "*",
          "percentage": 0,
          "allocation": []
        }
      ]
    },
    {
      "key": "foo",
      "bucketBy": "userId",
      "variations": [
        {
          "value": "control",
          "weight": 50
        },
        {
          "value": "treatment",
          "weight": 50,
          "variables": [
            {
              "key": "bar",
              "value": "bar_here",
              "overrides": [
                {
                  "segments": "{\"or\":[\"germany\",\"switzerland\"]}",
                  "value": "bar for DE or CH"
                }
              ]
            },
            {
              "key": "baz",
              "value": "baz_here",
              "overrides": [
                {
                  "segments": "netherlands",
                  "value": "baz for NL"
                }
              ]
            }
          ]
        }
      ],
      "traffic": [
        {
          "key": "1",
          "segments": "{\"and\":[\"mobile\",{\"or\":[\"germany\",\"switzerland\"]}]}",
          "percentage": 80000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                0,
                40000
              ]
            },
            {
              "variation": "treatment",
              "range": [
                40000,
                80000
              ]
            }
          ],
          "variables": {
            "qux": true
          }
        },
        {
          "key": "2",
          "segments": "*",
          "percentage": 50000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                0,
                25000
              ]
            },
            {
              "variation": "treatment",
              "range": [
                25000,
                50000
              ]
            }
          ]
        }
      ],
      "variablesSchema": [
        {
          "key": "bar",
          "type": "string",
          "defaultValue": ""
        },
        {
          "key": "baz",
          "type": "string",
          "defaultValue": ""
        },
        {
          "key": "qux",
          "type": "boolean",
          "defaultValue": false
        }
      ],
      "force": [
        {
          "conditions": {
            "and": [
              {
                "attribute": "userId",
                "operator": "equals",
                "value": "123"
              },
              {
                "attribute": "device",
                "operator": "equals",
                "value": "mobile"
              }
            ]
          },
          "variation": "treatment",
          "variables": {
            "bar": "yoooooo"
          }
        }
      ]
    },
    {
      "key": "footer",
      "bucketBy": "userId",
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 80000,
          "allocation": []
        }
      ]
    },
    {
      "key": "qux",
      "bucketBy": "userId",
      "variations": [
        {
          "value": "control",
          "weight": 33.34
        },
        {
          "value": "b",
          "weight": 33.33,
          "variables": [
            {
              "key": "fooConfig",
              "value": "{\"foo\": \"bar b\"}"
            }
          ]
        },
        {
          "value": "c",
          "weight": 33.33
        }
      ],
      "traffic": [
        {
          "key": "1",
          "segments": "[\"germany\"]",
          "percentage": 50000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                50001,
                66671
              ]
            },
            {
              "variation": "b",
              "range": [
                66671,
                83336
              ]
            },
            {
              "variation": "c",
              "range": [
                83336,
                100000
              ]
            }
          ],
          "variation": "b"
        },
        {
          "key": "2",
          "segments": "*",
          "percentage": 50000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                50001,
                66671
              ]
            },
            {
              "variation": "b",
              "range": [
                66671,
                83336
              ]
            },
            {
              "variation": "c",
              "range": [
                83336,
                100000
              ]
            }
          ]
        }
      ],
      "ranges": [
        [
          50001,
          100000
        ]
      ],
      "variablesSchema": [
        {
          "key": "fooConfig",
          "type": "json",
          "defaultValue": "{\"foo\": \"bar\"}"
        }
      ]
    },
    {
      "key": "redesign",
      "bucketBy": "userId",
      "traffic": [
        {
          "key": "1",
          "segments": "netherlands",
          "percentage": 100000,
          "allocation": []
        },
        {
          "key": "2",
          "segments": "*",
          "percentage": 0,
          "allocation": []
        }
      ]
    },
    {
      "key": "showBanner",
      "bucketBy": "userId",
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 0,
          "allocation": []
        }
      ]
    },
    {
      "key": "showHeader",
      "bucketBy": [
        "userId"
      ],
      "traffic": [
        {
          "key": "desktop",
          "segments": "[\"version_gt5\",\"desktop\"]",
          "percentage": 100000,
          "allocation": []
        },
        {
          "key": "mobile",
          "segments": "[\"mobile\"]",
          "percentage": 100000,
          "allocation": []
        },
        {
          "key": "all",
          "segments": "*",
          "percentage": 0,
          "allocation": []
        }
      ]
    },
    {
      "key": "showPopup",
      "bucketBy": "userId",
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 0,
          "allocation": []
        }
      ]
    },
    {
      "key": "sidebar",
      "bucketBy": "userId",
      "variations": [
        {
          "value": "control",
          "weight": 10
        },
        {
          "value": "treatment",
          "weight": 90,
          "variables": [
            {
              "key": "position",
              "value": "right"
            },
            {
              "key": "color",
              "value": "red",
              "overrides": [
                {
                  "segments": "[\"germany\"]",
                  "value": "yellow"
                },
                {
                  "segments": "[\"switzerland\"]",
                  "value": "white"
                }
              ]
            },
            {
              "key": "sections",
              "value": [
                "home",
                "about",
                "contact"
              ],
              "overrides": [
                {
                  "segments": "[\"germany\"]",
                  "value": [
                    "home",
                    "about",
                    "contact",
                    "imprint"
                  ]
                },
                {
                  "segments": "[\"netherlands\"]",
                  "value": [
                    "home",
                    "about",
                    "contact",
                    "bitterballen"
                  ]
                }
              ]
            }
          ]
        }
      ],
      "traffic": [
        {
          "key": "1",
          "segments": "*",
          "percentage": 100000,
          "allocation": [
            {
              "variation": "control",
              "range": [
                0,
                10000
              ]
            },
            {
              "variation": "treatment",
              "range": [
                10000,
                100000
              ]
            }
          ],
          "variables": {
            "title": "Sidebar Title for production"
          }
        }
      ],
      "variablesSchema": [
        {
          "key": "position",
          "type": "string",
          "defaultValue": "left"
        },
        {
          "key": "color",
          "type": "string",
          "defaultValue": "red"
        },
        {
          "key": "sections",
          "type": "array",
          "defaultValue": []
        },
        {
          "key": "title",
          "type": "string",
          "defaultValue": "Sidebar Title"
        }
      ]
    }
  ]
}
	`

	// Create a new instance of the SDK
	datafile, datafileErr := sdk.NewDatafileContent(datafileContent)
	if datafileErr != nil {
		fmt.Printf("Error creating datafile content: %v\n", datafileErr)
		return
	}
	instance, err := sdk.CreateInstance(sdk.InstanceOptions{
		Datafile: datafile,
	})
	if err != nil {
		fmt.Printf("Error creating SDK instance: %v\n", err)
		return
	}

	// Get the feature key from the --feature option
	featureKey, ok := parsedArgs.Named["feature"]
	if !ok {
		fmt.Println("Error: --feature option is required")
		return
	}

	// Parse the context from the --context option
	var context types.Context
	contextJSON, ok := parsedArgs.Named["context"]
	if ok {
		err := json.Unmarshal([]byte(contextJSON), &context)
		if err != nil {
			fmt.Printf("Error parsing context JSON: %v\n", err)
			return
		}
	}

	// Evaluate the feature
	evaluation := instance.EvaluateFlag(types.FeatureKey(featureKey), context)

	// Print the evaluation details
	fmt.Printf("Feature: %s\n", featureKey)
	fmt.Printf("Enabled: %v\n", evaluation.Enabled)
	fmt.Printf("Reason: %s\n", evaluation.Reason)
}
