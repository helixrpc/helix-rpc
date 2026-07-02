package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/helix-rpc/helix/compiler/ast"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokInt
	tokString
	tokPunct
)

type token struct {
	kind tokenKind
	val  string
}

type lexer struct {
	input []byte
	pos   int
}

func newLexer(input string) *lexer {
	return &lexer{input: []byte(input)}
}

func (l *lexer) nextToken() token {
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return token{kind: tokEOF}
	}

	ch := l.input[l.pos]
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		start := l.pos
		for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_' || l.input[l.pos] == '.') {
			l.pos++
		}
		return token{kind: tokIdent, val: string(l.input[start:l.pos])}
	}

	if unicode.IsDigit(rune(ch)) || ch == '-' {
		start := l.pos
		l.pos++
		for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
			l.pos++
		}
		return token{kind: tokInt, val: string(l.input[start:l.pos])}
	}

	if ch == '"' || ch == '\'' {
		quote := ch
		l.pos++
		start := l.pos
		for l.pos < len(l.input) && l.input[l.pos] != quote {
			l.pos++
		}
		val := string(l.input[start:l.pos])
		if l.pos < len(l.input) {
			l.pos++ // consume closing quote
		}
		return token{kind: tokString, val: val}
	}

	// Handle comments
	if ch == '/' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '/' {
		l.pos += 2
		for l.pos < len(l.input) && l.input[l.pos] != '\n' {
			l.pos++
		}
		return l.nextToken()
	}

	l.pos++
	return token{kind: tokPunct, val: string([]byte{ch})}
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
		} else {
			break
		}
	}
}

type protoParser struct {
	lex *lexer
	tok token
}

func ParseProto(content string) (*ast.AST, error) {
	p := &protoParser{lex: newLexer(content)}
	p.nextToken()
	return p.parseAST()
}

func (p *protoParser) nextToken() {
	p.tok = p.lex.nextToken()
}

func (p *protoParser) parseAST() (*ast.AST, error) {
	res := &ast.AST{
		Enums:    make([]*ast.EnumNode, 0),
		Structs:  make([]*ast.StructNode, 0),
		Services: make([]*ast.ServiceNode, 0),
	}

	for p.tok.kind != tokEOF {
		if p.tok.kind == tokIdent {
			switch p.tok.val {
			case "syntax":
				if err := p.parseSyntax(); err != nil {
					return nil, err
				}
			case "package":
				pkg, err := p.parsePackage()
				if err != nil {
					return nil, err
				}
				res.Namespace = pkg
			case "message":
				str, err := p.parseMessage()
				if err != nil {
					return nil, err
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
				p.nextToken() // Skip unknown top-level items for flexibility
			}
		} else {
			p.nextToken()
		}
	}

	return res, nil
}

func (p *protoParser) parseSyntax() error {
	p.nextToken() // consume 'syntax'
	if p.tok.kind != tokPunct || p.tok.val != "=" {
		return errors.New("expected '=' after syntax")
	}
	p.nextToken() // consume '='
	if p.tok.kind != tokString {
		return errors.New("expected string after syntax =")
	}
	p.nextToken() // consume string
	if p.tok.kind != tokPunct || p.tok.val != ";" {
		return errors.New("expected ';' after syntax definition")
	}
	p.nextToken() // consume ';'
	return nil
}

func (p *protoParser) parsePackage() (string, error) {
	p.nextToken() // consume 'package'
	if p.tok.kind != tokIdent {
		return "", errors.New("expected identifier after package")
	}
	pkg := p.tok.val
	p.nextToken() // consume package name
	if p.tok.kind != tokPunct || p.tok.val != ";" {
		return "", errors.New("expected ';' after package definition")
	}
	p.nextToken() // consume ';'
	return pkg, nil
}

func (p *protoParser) parseMessage() (*ast.StructNode, error) {
	p.nextToken() // consume 'message'
	if p.tok.kind != tokIdent {
		return nil, errors.New("expected identifier for message name")
	}
	name := p.tok.val
	p.nextToken() // consume name

	if p.tok.kind != tokPunct || p.tok.val != "{" {
		return nil, fmt.Errorf("expected '{' after message %s", name)
	}
	p.nextToken() // consume '{'

	str := &ast.StructNode{Name: name, Fields: []*ast.FieldNode{}}
	for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
		if p.tok.kind == tokIdent {
			// Check if message-level optional/repeated modifiers exist
			optional := false
			typeName := p.tok.val
			if typeName == "optional" {
				optional = true
				p.nextToken()
				typeName = p.tok.val
			} else if typeName == "repeated" {
				p.nextToken()
				typeName = "list<" + p.tok.val + ">" // Map list for simplicity
			}
			p.nextToken() // consume type name

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected field name identifier after type %s", typeName)
			}
			fieldName := p.tok.val
			p.nextToken() // consume field name

			if p.tok.kind != tokPunct || p.tok.val != "=" {
				return nil, fmt.Errorf("expected '=' after field name %s", fieldName)
			}
			p.nextToken() // consume '='

			if p.tok.kind != tokInt {
				return nil, fmt.Errorf("expected integer tag for field %s", fieldName)
			}
			tagVal, _ := strconv.Atoi(p.tok.val)
			p.nextToken() // consume tag value

			if p.tok.kind != tokPunct || p.tok.val != ";" {
				return nil, fmt.Errorf("expected ';' after field %s definition", fieldName)
			}
			p.nextToken() // consume ';'

			fType := parseTypeNode(typeName)
			str.Fields = append(str.Fields, &ast.FieldNode{
				Name:     fieldName,
				ID:       int32(tagVal),
				Type:     fType,
				Optional: optional,
			})
		} else {
			p.nextToken()
		}
	}

	if p.tok.kind != tokPunct || p.tok.val != "}" {
		return nil, fmt.Errorf("expected '}' to close message %s", name)
	}
	p.nextToken() // consume '}'
	return str, nil
}

func (p *protoParser) parseEnum() (*ast.EnumNode, error) {
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
	for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
		if p.tok.kind == tokIdent {
			enumKey := p.tok.val
			p.nextToken() // consume key

			if p.tok.kind != tokPunct || p.tok.val != "=" {
				return nil, fmt.Errorf("expected '=' after enum key %s", enumKey)
			}
			p.nextToken() // consume '='

			if p.tok.kind != tokInt {
				return nil, fmt.Errorf("expected integer value for enum key %s", enumKey)
			}
			val, _ := strconv.Atoi(p.tok.val)
			p.nextToken() // consume integer

			if p.tok.kind != tokPunct || p.tok.val != ";" {
				return nil, fmt.Errorf("expected ';' after enum key %s", enumKey)
			}
			p.nextToken() // consume ';'

			en.Values[enumKey] = int32(val)
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

func (p *protoParser) parseService() (*ast.ServiceNode, error) {
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
		if p.tok.kind == tokIdent && p.tok.val == "rpc" {
			p.nextToken() // consume 'rpc'
			if p.tok.kind != tokIdent {
				return nil, errors.New("expected method name identifier")
			}
			methodName := p.tok.val
			p.nextToken() // consume method name

			if p.tok.kind != tokPunct || p.tok.val != "(" {
				return nil, fmt.Errorf("expected '(' after method %s", methodName)
			}
			p.nextToken() // consume '('

			clientStreaming := false
			if p.tok.kind == tokIdent && p.tok.val == "stream" {
				clientStreaming = true
				p.nextToken()
			}

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected input type for method %s", methodName)
			}
			inputType := p.tok.val
			p.nextToken() // consume input type

			if p.tok.kind != tokPunct || p.tok.val != ")" {
				return nil, fmt.Errorf("expected ')' after input type of method %s", methodName)
			}
			p.nextToken() // consume ')'

			if p.tok.kind != tokIdent || p.tok.val != "returns" {
				return nil, fmt.Errorf("expected 'returns' keyword for method %s", methodName)
			}
			p.nextToken() // consume 'returns'

			if p.tok.kind != tokPunct || p.tok.val != "(" {
				return nil, fmt.Errorf("expected '(' before output type of method %s", methodName)
			}
			p.nextToken() // consume '('

			serverStreaming := false
			if p.tok.kind == tokIdent && p.tok.val == "stream" {
				serverStreaming = true
				p.nextToken()
			}

			if p.tok.kind != tokIdent {
				return nil, fmt.Errorf("expected output type for method %s", methodName)
			}
			outputType := p.tok.val
			p.nextToken() // consume output type

			if p.tok.kind != tokPunct || p.tok.val != ")" {
				return nil, fmt.Errorf("expected ')' after output type of method %s", methodName)
			}
			p.nextToken() // consume ')'

			// Allow optional semicolon or options blocks { }
			restMethod := ""
			restPath := ""
			if p.tok.kind == tokPunct && p.tok.val == ";" {
				p.nextToken() // consume ';'
			} else if p.tok.kind == tokPunct && p.tok.val == "{" {
				p.nextToken() // consume '{'
				for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
					if p.tok.kind == tokIdent && p.tok.val == "option" {
						p.nextToken() // consume 'option'
						if p.tok.kind == tokPunct && p.tok.val == "(" {
							p.nextToken()
							if p.tok.kind == tokIdent && p.tok.val == "google.api.http" {
								p.nextToken()
								if p.tok.kind == tokPunct && p.tok.val == ")" {
									p.nextToken()
									if p.tok.kind == tokPunct && p.tok.val == "=" {
										p.nextToken()
										if p.tok.kind == tokPunct && p.tok.val == "{" {
											p.nextToken()
											for p.tok.kind != tokEOF && !(p.tok.kind == tokPunct && p.tok.val == "}") {
												if p.tok.kind == tokIdent {
													verb := p.tok.val
													p.nextToken()
													if p.tok.kind == tokPunct && p.tok.val == ":" {
														p.nextToken()
														if p.tok.kind == tokString {
															pathVal := p.tok.val
															p.nextToken()
															restMethod = strings.ToUpper(verb)
															restPath = pathVal
														}
													}
												} else {
													p.nextToken()
												}
											}
											if p.tok.kind == tokPunct && p.tok.val == "}" {
												p.nextToken() // consume '}"
											}
										}
									}
								}
							}
						}
						if p.tok.kind == tokPunct && p.tok.val == ";" {
							p.nextToken()
						}
					} else {
						p.nextToken()
					}
				}
				if p.tok.kind == tokPunct && p.tok.val == "}" {
					p.nextToken() // consume '}'
				} else {
					return nil, fmt.Errorf("expected closing brace '}' for method %s", methodName)
				}
			}

			srv.Methods = append(srv.Methods, &ast.MethodNode{
				Name:            methodName,
				InputType:       inputType,
				OutputType:      outputType,
				ClientStreaming: clientStreaming,
				ServerStreaming: serverStreaming,
				RESTMethod:      restMethod,
				RESTPath:        restPath,
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

func parseTypeNode(tStr string) ast.TypeNode {
	tStr = strings.TrimSpace(tStr)
	if strings.HasPrefix(tStr, "list<") && strings.HasSuffix(tStr, ">") {
		subTypeStr := tStr[5 : len(tStr)-1]
		sub := parseTypeNode(subTypeStr)
		return ast.TypeNode{
			Kind:      ast.TypeList,
			ValueType: &sub,
		}
	}
	if strings.HasPrefix(tStr, "map<") && strings.HasSuffix(tStr, ">") {
		parts := strings.SplitN(tStr[4:len(tStr)-1], ",", 2)
		if len(parts) == 2 {
			k := parseTypeNode(parts[0])
			v := parseTypeNode(parts[1])
			return ast.TypeNode{
				Kind:      ast.TypeMap,
				KeyType:   &k,
				ValueType: &v,
			}
		}
	}

	switch tStr {
	case "int32", "i32":
		return ast.TypeNode{Kind: ast.TypeInt32}
	case "int64", "i64":
		return ast.TypeNode{Kind: ast.TypeInt64}
	case "string":
		return ast.TypeNode{Kind: ast.TypeString}
	case "bytes", "binary":
		return ast.TypeNode{Kind: ast.TypeBinary}
	case "bool":
		return ast.TypeNode{Kind: ast.TypeBool}
	case "double":
		return ast.TypeNode{Kind: ast.TypeDouble}
	default:
		// Custom message/struct type or enum reference
		return ast.TypeNode{Kind: ast.TypeStruct, Name: tStr}
	}
}
