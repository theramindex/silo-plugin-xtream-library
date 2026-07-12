package config

import (
	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

type ConfigSchema = pluginv1.ConfigSchema

const connectionJSONSchema = `{
  "type": "object",
  "properties": {
    "source_mode": {
      "type": "string",
      "enum": ["xtream", "m3u_xmltv"],
      "default": "xtream"
    },
    "base_url": {
      "type": "string",
      "format": "uri"
    },
    "username": {
      "type": "string"
    },
    "password": {
      "type": "string",
      "writeOnly": true
    },
    "m3u_url": {
      "type": "string",
      "format": "uri"
    },
	    "epg_xml_url": {
	      "type": "string",
	      "format": "uri"
	    }
  },
  "required": ["source_mode"],
  "additionalProperties": false,
  "allOf": [
    {
      "if": {
        "properties": {
          "source_mode": {
            "const": "xtream"
          }
        }
      },
      "then": {
        "required": ["base_url", "username", "password"]
      }
    },
    {
      "if": {
        "properties": {
          "source_mode": {
            "const": "m3u_xmltv"
          }
        }
      },
      "then": {
        "required": ["m3u_url", "epg_xml_url"]
      }
    }
  ]
}`

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
	return []*ConfigSchema{
		objectSchema("connection", "Xtreme Codes for Silo", "Choose the upstream source. Xtream Codes is the full-featured primary source. M3U + XMLTV provides compatible live TV and guide behavior.", connectionJSONSchema, true, []*pluginv1.AdminFormField{
			{Key: "source_mode", Label: "Source Type", Description: "Xtream Codes is the primary source. M3U + XMLTV is a secondary live TV and guide source.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_SELECT, DefaultValue: structpb.NewStringValue(string(SourceModeXtream)), Options: []*pluginv1.AdminFormOption{
				{Value: string(SourceModeXtream), Label: "Xtream Codes", Description: "Fill Server URL, Username, and Password with the provider credentials for the shared Xtream Account."},
				{Value: string(SourceModeM3UXMLTV), Label: "M3U + XMLTV", Description: "Fill M3U Playlist URL and Custom XMLTV URL. Xtream-only VOD, series, and catch-up are unavailable."},
			}},
			{Key: "base_url", Label: "Server URL", Description: "Required for Xtream Codes. Use the provider base URL, not the full player_api.php URL.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT, Placeholder: "https://provider.example.com"},
			{Key: "username", Label: "Username", Description: "Required for Xtream Codes.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT},
			{Key: "password", Label: "Password", Description: "Required for Xtream Codes and stored as a Silo plugin secret.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_PASSWORD, Secret: true},
			{Key: "m3u_url", Label: "M3U Playlist URL", Description: "Required only for M3U + XMLTV.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT, Placeholder: "https://provider.example.com/playlist.m3u"},
			{Key: "epg_xml_url", Label: "XMLTV URL", Description: "Required only for M3U + XMLTV.", Control: pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT, Placeholder: "https://provider.example.com/guide.xml"},
		}, "Save Xtreme Codes for Silo settings"),
		objectSchema("category_settings", "Live TV Admin Settings", "Admin-managed Live TV organization, recording, event, and ECM settings.", categorySettingsJSONSchema, false, []*pluginv1.AdminFormField{}, "Save admin settings"),
	}
}

func UserConfigSchema() []*ConfigSchema {
	return []*ConfigSchema{
		objectSchema("preferences", "Preferences", "Per-user Dispatcharr plugin preferences stored by Silo.", `{"type":"object","properties":{"favorites":{"type":"object","additionalProperties":{"type":"boolean"}},"favoriteOrder":{"type":"array","items":{"type":"string"}},"autoFavorites":{"type":"object","additionalProperties":{"type":"boolean"}},"hiddenCategories":{"type":"object","additionalProperties":{"type":"boolean"}},"sportsFavoriteTeams":{"type":"object","additionalProperties":{"type":"boolean"}},"keywordPasses":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"keyword":{"type":"string"},"createdAt":{"type":"integer"}},"required":["id","keyword"],"additionalProperties":false}},"recentSearches":{"type":"array","items":{"type":"string"}},"recentChannels":{"type":"array","items":{"type":"string"}},"continueWatching":{"type":"object","additionalProperties":true},"playback":{"type":"object","additionalProperties":true},"categoryParsing":{"type":"object","properties":{"enabled":{"type":"boolean"},"mode":{"type":"string","enum":["off","delimiter","regex"]},"delimiter":{"type":"string","enum":["dash","pipe"]},"regex":{"type":"string"},"output":{"type":"string"}},"additionalProperties":false},"profileSelection":{"type":"object","properties":{"mode":{"type":"string","enum":["all","selected"]},"profileIds":{"type":"array","items":{"type":"string"},"uniqueItems":true}},"required":["mode","profileIds"],"additionalProperties":false},"customGroups":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"order":{"type":"integer"}},"required":["id","name"],"additionalProperties":false}},"customGroupMemberships":{"type":"object","additionalProperties":{"type":"array","items":{"type":"string"}}}},"additionalProperties":false}`, false, []*pluginv1.AdminFormField{}, "Save preferences"),
		objectSchema("adminCategorySettings", "Admin Category Settings", "Admin-managed Live TV category mode saved through Silo plugin settings.", categorySettingsJSONSchema, false, []*pluginv1.AdminFormField{}, "Save category settings"),
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
