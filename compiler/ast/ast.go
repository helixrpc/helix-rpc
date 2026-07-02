package ast

// Type represents the unified types supported by Helix RPC.
type Type string

const (
	TypeInt32   Type = "T_INT32"
	TypeInt64   Type = "T_INT64"
	TypeString  Type = "T_STRING"
	TypeBinary  Type = "T_BINARY"
	TypeBool    Type = "T_BOOL"
	TypeDouble  Type = "T_DOUBLE"
	TypeVoid    Type = "T_VOID"
	TypeMap     Type = "T_MAP"
	TypeList    Type = "T_LIST"
	TypeStruct  Type = "T_STRUCT"
	TypeEnum    Type = "T_ENUM"
)

// TypeNode represents a type in the AST.
type TypeNode struct {
	Kind      Type
	Name      string     // Used when Kind is T_STRUCT or T_ENUM
	KeyType   *TypeNode  // Used when Kind is T_MAP
	ValueType *TypeNode  // Used when Kind is T_MAP or T_LIST
}

// FieldNode represents a field in a struct.
type FieldNode struct {
	Name       string
	ID         int32 // Unified Field ID (Thrift ID / Proto tag)
	Type       TypeNode
	Optional   bool
	DefaultVal interface{}
}

// StructNode represents a data structure.
type StructNode struct {
	Name   string
	Fields []*FieldNode
}

// EnumNode represents a sequence of key-value integer enums.
type EnumNode struct {
	Name   string
	Values map[string]int32
}

// MethodNode represents an RPC service method.
type MethodNode struct {
	Name            string
	InputType       string
	OutputType      string
	ClientStreaming bool
	ServerStreaming bool
	RESTMethod      string
	RESTPath        string
}

// ServiceNode represents an RPC service contract.
type ServiceNode struct {
	Name    string
	Methods []*MethodNode
}

// AST is the root node of the Unified AST representation.
type AST struct {
	Namespace string
	Enums     []*EnumNode
	Structs   []*StructNode
	Services  []*ServiceNode
}
