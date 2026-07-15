package config

import (
	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

type ConfigSchema = pluginv1.ConfigSchema

const categorySettingsJSONSchema = `{
  "type": "object",
  "properties": {
    "mode": {
      "type": "string",
      "enum": ["normal", "delimiter"],
      "default": "normal"
    },
    "delimiter": {
      "type": "string",
      "enum": ["pipe", "dash"],
      "default": "pipe"
    },
    "virtualGroupLabel": {
      "type": "string",
      "default": "Virtual Groups"
    },
    "ecmEnabled": {
      "type": "boolean",
      "default": false
    },
    "ecmURL": {
      "type": "string",
      "default": ""
    },
    "allowRecordingsByDefault": {
      "type": "boolean",
      "default": true
    },
    "sportsFirstPlayerEnabled": {
      "type": "boolean",
      "default": false
    },
    "liveRewindEnabled": {
      "type": "boolean",
      "default": false
    },
    "liveRewindCacheGB": {
      "type": "number",
      "minimum": 1,
      "maximum": 500,
      "default": 5
    },
    "liveRewindWindowMinutes": {
      "type": "integer",
      "enum": [15, 30, 60, 90, 120],
      "default": 30
    },
    "liveRewindMinFreeGB": {
      "type": "number",
      "minimum": 1,
      "maximum": 100,
      "default": 2
    },
    "liveRewindMaxChannels": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100,
      "default": 20
    },
    "virtualGroupSource": {
      "type": "string",
      "enum": ["group", "group_channel", "profile_group", "channel"],
      "default": "group"
    },
    "collapseDuplicateVirtualGroups": {
      "type": "boolean",
      "default": true
    },
    "inferChannelNameGroups": {
      "type": "boolean",
      "default": false
    },
    "categoryRenames": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "sourcePath": {
            "type": "string",
            "minLength": 1
          },
          "displayName": {
            "type": "string",
            "minLength": 1
          }
        },
        "required": ["sourcePath", "displayName"],
        "additionalProperties": false
      },
      "default": []
    },
    "categoryAliases": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "sourcePath": {
            "type": "string",
            "minLength": 1
          },
          "aliasPath": {
            "type": "string",
            "minLength": 1
          }
        },
        "required": ["sourcePath", "aliasPath"],
        "additionalProperties": false
      },
      "default": []
    },
    "eventKeywords": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "categoryId": {
            "type": "string",
            "minLength": 1
          },
          "categoryName": {
            "type": "string",
            "minLength": 1
          },
          "keywords": {
            "type": "array",
            "items": {
              "type": "string",
              "minLength": 1
            }
          },
          "excludeKeywords": {
            "type": "array",
            "items": {
              "type": "string",
              "minLength": 1
            },
            "default": []
          },
          "eventSeries": {
            "type": "boolean",
            "default": false
          },
          "groupWindowMinutes": {
            "type": "integer",
            "minimum": 15,
            "maximum": 360,
            "default": 60
          }
        },
        "required": ["categoryId", "categoryName", "keywords"],
        "additionalProperties": false
      },
      "default": []
    }
  },
  "additionalProperties": false
}`

func GlobalConfigSchema() []*ConfigSchema {
	return nil
}

func UserConfigSchema() []*ConfigSchema {
	return []*ConfigSchema{
		objectSchema("preferences", "Preferences", "Per-user Xtreme browsing preferences stored by Silo.", `{"type":"object","properties":{"favorites":{"type":"object","additionalProperties":{"type":"boolean"}},"favoriteOrder":{"type":"array","items":{"type":"string"}},"hiddenCategories":{"type":"object","additionalProperties":{"type":"boolean"}},"groupCategoriesByPipe":{"type":"boolean"},"recentSearches":{"type":"array","items":{"type":"string"}},"recentChannels":{"type":"array","items":{"type":"string"}},"continueWatching":{"type":"object","additionalProperties":true},"playback":{"type":"object","additionalProperties":true},"customGroups":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"order":{"type":"integer"}},"required":["id","name"],"additionalProperties":false}},"customGroupMemberships":{"type":"object","additionalProperties":{"type":"array","items":{"type":"string"}}}},"additionalProperties":false}`, false, []*pluginv1.AdminFormField{}, "Save preferences"),
	}
}

func objectSchema(key, title, description, jsonSchema string, required bool, fields []*pluginv1.AdminFormField, submitLabel string) *ConfigSchema {
	return &pluginv1.ConfigSchema{
		Key:         key,
		Title:       title,
		Description: description,
		JsonSchema:  jsonSchema,
		Required:    required,
		AdminForm:   &pluginv1.AdminFormDescriptor{Fields: fields, SubmitLabel: submitLabel},
	}
}
