package parser

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/helix-rpc/helix/compiler/ast"
)

type thriftParser struct {
	lex *lexer
	tok token
}

func ParseThrift(content string) (*ast.AST, error) {
	p := &thriftParser{lex: newLexer(content)}
	p.nextToken()
	return p.parseAST()
}

func (p *thriftParser) nextToken() {
	p.tok = p.lex.nextToken()
}

func (p *thriftParser) parseAST() (*ast.AST, error) {
	res := &ast.AST{
		Enums:    make([]*ast.EnumNode, 0),
		Structs:  make([]*ast.StructNode, 0),
		Services: make([]*ast.ServiceNode, 0),
	}

	for p.tok.kind != tokEOF {
		if p.tok.kind == tokIdent {
			switch p.tok.val {
			case "namespace":
				ns, err := p.parseNamespace()
				if err != nil {
					return nil, err
				}
				if res.Namespace == "" {
					res.Namespace = ns // Capture first valid namespace name as default
				}
			case "struct", "union", "exception":
				blockType := p.tok.val
				str, err := p.parseStruct()
				if err != nil {
					return nil, err
				}
				if blockType == "union" || blockType == "exception" {
					str.HasFallback = true
				}
				res.Structs = append(res.Structs, str)
			case "enum":
				en, err := p.parseEnum()
				if err != nil {
					return nil, err
				}
				res.Enums = append(res.Enums, en)
			case "service":
				srv, err := p.parseService()
				if err != nil {
					return nil, err
				}
				res.Services = append(res.Services, srv)
			default:
				p.nextToken() // Skip unknown top-level tokens
			}
		} else {
			p.nextToken()
		}
	}

	return res, nil
}

func (p *thriftParser) parseNamespace() (string, error) {
	p.nextToken() // consume 'namespace'
	if p.tok.kind != tokIdent {
		return "", errors.New("expected language target or '*' after namespace")
	}
	p.nextToken() // consume scope / '*'

	if p.tok.kind != tokIdent {
		return "", errors.New("expected namespace identifier")
	}
	ns := p.tok.val
	p.nextToken() // consume namespace name
	return ns, nil
}

func (p *thriftParser) parseStruct() (*ast.StructNode, error) {
	p.nextToken() // consume 'struct'
	if p.tok.kind != tokIdent {
		return nil, errors.New("expected identifier for struct name")
	}
	name := p.tok.val
	p.nextToken() // consume name

	if p.tok.kind != tokPunct || p.tok.val != "{" {
		return nil, fmt.Errorf("expected '{' after struct %s", name)
	}
	p.nextToken() // consume '{'

	str := &ast.StructNode{Name: name, Fields: []*ast.FieldNode{}, HasFallback: false}
	for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
		if p.tok.kind == tokInt {
			// Field specification: [FieldID]: [required/optional] [Type] [Name]
			fieldId, _ := strconv.Atoi(p.tok.val)
			p.nextToken() // consume integer

			if p.tok.kind != tokPunct || p.tok.val != ":" {
				return nil, fmt.Errorf("expected ':' after field ID %d in struct %s", fieldId, name)
			}
			p.nextToken() // consume ':'

			optional := false
			if p.tok.kind == tokIdent {
				if p.tok.val == "optional" {
					optional = true
					p.nextToken()
				} else if p.tok.val == "required" {
					p.nextToken()
				}
			}

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected type name for field in struct %s", name)
			}
			typeName := p.tok.val
			p.nextToken() // consume type name

			// Check list/map parameters (e.g. list<i64>)
			if typeName == "list" && p.tok.kind == tokPunct && p.tok.val == "<" {
				p.nextToken() // consume '<'
				if p.tok.kind != tokIdent {
					return nil, errors.New("expected list item type")
				}
				subType := p.tok.val
				p.nextToken() // consume list type
				if p.tok.kind != tokPunct || p.tok.val != ">" {
					return nil, errors.New("expected '>' to close list type")
				}
				p.nextToken() // consume '>'
				typeName = "list<" + subType + ">"
			} else if typeName == "map" && p.tok.kind == tokPunct && p.tok.val == "<" {
				p.nextToken() // consume '<'
				p.nextToken() // key type
				if p.tok.kind == tokPunct && p.tok.val == "," {
					p.nextToken()
				}
				p.nextToken() // value type
				if p.tok.kind == tokPunct && p.tok.val == ">" {
					p.nextToken() // consume '>'
				}
				typeName = "map"
			}

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected field name identifier in struct %s", name)
			}
			fieldName := p.tok.val
			p.nextToken() // consume field name

			// Optional comma/semicolon delimiter
			if p.tok.kind == tokPunct && (p.tok.val == ";" || p.tok.val == ",") {
				p.nextToken() // consume delimiter
			}

			fType := parseTypeNode(typeName)
			if fType.Kind == ast.TypeMap {
				str.HasFallback = true
			}
			str.Fields = append(str.Fields, &ast.FieldNode{
				Name:     fieldName,
				ID:       int32(fieldId),
				Type:     fType,
				Optional: optional,
			})
		} else {
			p.nextToken()
		}
	}

	if p.tok.kind != tokPunct || p.tok.val != "}" {
		return nil, fmt.Errorf("expected '}' to close struct %s", name)
	}
	p.nextToken() // consume '}'
	return str, nil
}

func (p *thriftParser) parseEnum() (*ast.EnumNode, error) {
	p.nextToken() // consume 'enum'
	if p.tok.kind != tokIdent {
		return nil, errors.New("expected identifier for enum name")
	}
	name := p.tok.val
	p.nextToken() // consume name

	if p.tok.kind != tokPunct || p.tok.val != "{" {
		return nil, fmt.Errorf("expected '{' after enum %s", name)
	}
	p.nextToken() // consume '{'

	en := &ast.EnumNode{Name: name, Values: make(map[string]int32)}
	var currentVal int32 = 0
	for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
		if p.tok.kind == tokIdent {
			enumKey := p.tok.val
			p.nextToken() // consume key

			if p.tok.kind == tokPunct && p.tok.val == "=" {
				p.nextToken() // consume '='
				if p.tok.kind != tokInt {
					return nil, fmt.Errorf("expected integer value for enum key %s", enumKey)
				}
				val, _ := strconv.Atoi(p.tok.val)
				currentVal = int32(val)
				p.nextToken() // consume integer
			}

			en.Values[enumKey] = currentVal
			currentVal++

			// Optional comma/semicolon delimiter
			if p.tok.kind == tokPunct && (p.tok.val == ";" || p.tok.val == ",") {
				p.nextToken()
			}
		} else {
			p.nextToken()
		}
	}

	if p.tok.kind != tokPunct || p.tok.val != "}" {
		return nil, fmt.Errorf("expected '}' to close enum %s", name)
	}
	p.nextToken() // consume '}'
	return en, nil
}

func (p *thriftParser) parseService() (*ast.ServiceNode, error) {
	p.nextToken() // consume 'service'
	if p.tok.kind != tokIdent {
		return nil, errors.New("expected identifier for service name")
	}
	name := p.tok.val
	p.nextToken() // consume name

	if p.tok.kind != tokPunct || p.tok.val != "{" {
		return nil, fmt.Errorf("expected '{' after service %s", name)
	}
	p.nextToken() // consume '{'

	srv := &ast.ServiceNode{Name: name, Methods: []*ast.MethodNode{}}
	for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
		if p.tok.kind == tokIdent {
			// Thrift Method definition: [OutputType] [MethodName](1: [InputType] [reqName])
			outputType := p.tok.val
			p.nextToken() // consume output type

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected method name identifier in service %s", name)
			}
			methodName := p.tok.val
			p.nextToken() // consume method name

			if p.tok.kind != tokPunct || p.tok.val != "(" {
				return nil, fmt.Errorf("expected '(' after method name %s", methodName)
			}
			p.nextToken() // consume '('

			var inputType string
			if p.tok.kind == tokInt {
				// Param ID
				p.nextToken() // consume integer
				if p.tok.kind == tokPunct && p.tok.val == ":" {
					p.nextToken() // consume ':'
				}
			}

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected input type in method %s arguments", methodName)
			}
			inputType = p.tok.val
			p.nextToken() // consume input type

			if p.tok.kind == tokIdent {
				p.nextToken() // skip param name if present (e.g. request/req)
			}

			if p.tok.kind != tokPunct || p.tok.val != ")" {
				return nil, fmt.Errorf("expected ')' after arguments of method %s", methodName)
			}
			p.nextToken() // consume ')'

			// Optional delimiter (semicolon or comma)
			if p.tok.kind == tokPunct && (p.tok.val == ";" || p.tok.val == ",") {
				p.nextToken()
			}

			srv.Methods = append(srv.Methods, &ast.MethodNode{
				Name:            methodName,
				InputType:       inputType,
				OutputType:      outputType,
				ClientStreaming: false,
				ServerStreaming: false,
			})
		} else {
			p.nextToken()
		}
	}

	if p.tok.kind != tokPunct || p.tok.val != "}" {
		return nil, fmt.Errorf("expected '}' to close service %s", name)
	}
	p.nextToken() // consume '}'
	return srv, nil
}
