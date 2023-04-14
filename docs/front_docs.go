// Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplatefront = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/articles": {
            "get": {
                "description": "get articles",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "articles"
                ],
                "summary": "List articles",
                "parameters": [
                    {
                        "minimum": 0,
                        "type": "integer",
                        "default": 0,
                        "description": "Page Index",
                        "name": "index",
                        "in": "query"
                    },
                    {
                        "maximum": 100,
                        "minimum": 1,
                        "type": "integer",
                        "default": 20,
                        "description": "Page Size",
                        "name": "size",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Sites",
                        "name": "sites",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Languages",
                        "name": "langs",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "format": "date-time",
                        "description": "From DateTime",
                        "name": "dt",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/pubsub.Article"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    }
                }
            }
        },
        "/realtime": {
            "get": {
                "description": "sse stream",
                "tags": [
                    "sse"
                ],
                "summary": "Realtime",
                "responses": {
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    },
                    "default": {
                        "description": ""
                    }
                }
            }
        },
        "/sites": {
            "get": {
                "description": "get sites",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "sites"
                ],
                "summary": "List sites",
                "parameters": [
                    {
                        "minimum": 0,
                        "type": "integer",
                        "default": 0,
                        "description": "Page Index",
                        "name": "index",
                        "in": "query"
                    },
                    {
                        "maximum": 100,
                        "minimum": 1,
                        "type": "integer",
                        "default": 20,
                        "description": "Page Size",
                        "name": "size",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/front.Site"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/wool.Error"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "front.Site": {
            "type": "object",
            "properties": {
                "domain": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "languages": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "pubsub.Article": {
            "type": "object",
            "properties": {
                "categories": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "id": {
                    "type": "string"
                },
                "image": {
                    "type": "string"
                },
                "lang": {
                    "type": "string"
                },
                "link": {
                    "type": "string"
                },
                "long_desc": {
                    "type": "string"
                },
                "pub_date": {
                    "type": "string"
                },
                "relative_pub_date": {
                    "type": "string"
                },
                "short_desc": {
                    "type": "string"
                },
                "site_id": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "wool.Error": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "integer"
                },
                "data": {},
                "developer_message": {
                    "type": "string"
                },
                "message": {
                    "type": "string"
                }
            }
        }
    }
}`

// SwaggerInfofront holds exported Swagger Info so clients can modify it
var SwaggerInfofront = &swag.Spec{
	Version:          "1.0",
	Host:             "",
	BasePath:         "/api/v1",
	Schemes:          []string{},
	Title:            "Rumors Frontend API",
	Description:      "",
	InfoInstanceName: "front",
	SwaggerTemplate:  docTemplatefront,
}

func init() {
	swag.Register(SwaggerInfofront.InstanceName(), SwaggerInfofront)
}
