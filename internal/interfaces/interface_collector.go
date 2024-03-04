package interfaces

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/tools/go/packages"
)

var errNoFiles = errors.New("no go files in package")

type Token string

type Interface struct {
	Name               string
	DefaultPackageName string
	ImportPath         string
}

type Collector struct {
	loadDir string
}

func NewCollector(loadDir string) *Collector {
	return &Collector{
		loadDir: loadDir,
	}
}

func (c *Collector) Collect(f *ast.File, fs []*ast.File) (map[Token]Interface, error) {
	mapFromImports, err := c.collectFromImports(f)
	if err != nil {
		return nil, err
	}

	mapFromPackageFiles, err := c.collectFromPackageFiles(fs)
	if err != nil {
		return nil, err
	}

	resultMap := mapFromImports
	for k, v := range mapFromPackageFiles {
		resultMap[k] = v
	}

	return resultMap, nil
}

func (c *Collector) collectFromPackageFiles(fs []*ast.File) (map[Token]Interface, error) {
	pkg, err := c.loadPackage(".")
	if pkg == nil {
		return nil, fmt.Errorf("failed to load current package: %w", err)
	}

	resultMap := make(map[Token]Interface)
	ifaces := c.findAllInterfaces(fs)
	for _, iface := range ifaces {
		resultMap[Token(iface.Name.Name)] = Interface{
			Name:               iface.Name.Name,
			DefaultPackageName: pkg.Name,
			ImportPath:         pkg.PkgPath,
		}
	}

	return resultMap, nil
}

func (c *Collector) collectFromImports(f *ast.File) (map[Token]Interface, error) {
	wg := sync.WaitGroup{}
	wg.Add(len(f.Imports))
	mapChan := make(chan map[Token]Interface, len(f.Imports))

	var resErr atomic.Value
	for _, imp := range f.Imports {
		go func(imp *ast.ImportSpec) {
			defer wg.Done()

			importMap, err := c.collectFromImport(imp)

			if err != nil && resErr.Load() == nil {
				resErr.Store(err)
			}

			mapChan <- importMap
		}(imp)
	}

	go func() {
		wg.Wait()
		close(mapChan)
	}()

	resultMap := make(map[Token]Interface)
	for m := range mapChan {
		for k, v := range m {
			resultMap[k] = v
		}
	}

	var err error
	if e := resErr.Load(); e != nil {
		err = e.(error)
	}

	return resultMap, err
}

func (c *Collector) collectFromImport(imp *ast.ImportSpec) (map[Token]Interface, error) {
	path := strings.Trim(imp.Path.Value, "\"")
	pkg, err := c.loadPackage(path)
	// we dont want to check error here
	// because even if there is any error
	// package might be filled
	if pkg == nil {
		return nil, fmt.Errorf("cant load package: %w", err)
	}

	astPkg, err := c.loadAST(pkg)
	if errors.Is(err, errNoFiles) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cant load ast of package %s: %w", pkg.ID, err)
	}

	files := c.convertFilesMap(astPkg.Files)
	ifaces := c.findAllInterfaces(files)

	resultMap := make(map[Token]Interface)
	for _, iface := range ifaces {
		interfaceToken, isMade := c.makeToken(imp.Name, iface, pkg.Name)
		if !isMade {
			continue
		}

		resultMap[interfaceToken] = Interface{
			DefaultPackageName: pkg.Name,
			Name:               iface.Name.Name,
			ImportPath:         pkg.PkgPath,
		}
	}

	return resultMap, nil
}

func (c *Collector) convertFilesMap(fileMap map[string]*ast.File) []*ast.File {
	files := make([]*ast.File, 0, len(fileMap))
	for _, file := range fileMap {
		files = append(files, file)
	}
	return files
}

func (c *Collector) makeToken(importIdent *ast.Ident, spec *ast.TypeSpec, defaultPackage string) (Token, bool) {
	// if interface is not exported do not collect it
	if !ast.IsExported(spec.Name.Name) {
		return "", false
	}

	// if import has some name, use it to make token
	if importIdent != nil {
		return Token(fmt.Sprintf("%s.%s", importIdent.Name, spec.Name.Name)), true
	}

	// if import does not have specified name, use default name
	return Token(fmt.Sprintf("%s.%s", defaultPackage, spec.Name.Name)), true
}

// code below copied from https://github.com/gojuno/minimock/blob/master/cmd/minimock/minimock.go#L90
func (c *Collector) findAllInterfaces(fileset []*ast.File) []*ast.TypeSpec {
	// Find all declared types in a single package
	types := make([]*ast.TypeSpec, 0)
	for _, file := range fileset {
		types = append(types, c.findAllTypeSpecsInFile(file)...)
	}

	// Filter interfaces from all the declarations
	interfaces := make([]*ast.TypeSpec, 0)
	for _, typeSpec := range types {
		if isInterface(typeSpec) {
			interfaces = append(interfaces, typeSpec)
		}
	}

	return interfaces
}

func isInterface(typeSpec *ast.TypeSpec) bool {
	// Check if this type declaration is specifically an interface declaration
	_, ok := typeSpec.Type.(*ast.InterfaceType)
	return ok
}

// findAllInterfaceNodesInFile ranges over file's AST nodes and extracts all interfaces inside
// returned *ast.TypeSpecs can be safely interpreted as interface declaration nodes
func (c *Collector) findAllTypeSpecsInFile(f *ast.File) []*ast.TypeSpec {
	typeSpecs := []*ast.TypeSpec{}

	// Range over all declarations in a single file
	for _, declaration := range f.Decls {
		// Check if declaration is an import, constant, type or variable declaration.
		// If it is, check specifically if it's a TYPE as all interfaces are types
		if genericDeclaration, ok := declaration.(*ast.GenDecl); ok && genericDeclaration.Tok == token.TYPE {
			// Range over all specifications and find ones that are Type declarations
			// This is mostly a precaution
			for _, spec := range genericDeclaration.Specs {
				// Check directly for a type spec declaration
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					typeSpecs = append(typeSpecs, typeSpec)
				}
			}
		}
	}

	return typeSpecs
}

func (c *Collector) loadAST(p *packages.Package) (*ast.Package, error) {
	fs := token.NewFileSet()

	if len(p.GoFiles) == 0 {
		return nil, errNoFiles
	}
	dir := filepath.Dir(p.GoFiles[0])

	pkgs, err := parser.ParseDir(fs, dir, nil, parser.DeclarationErrors|parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if ap, ok := pkgs[p.Name]; ok {
		return ap, nil
	}

	return nil, fmt.Errorf("package with name %s not found in parsed directory", p.Name)
}

func (c *Collector) loadPackage(importPath string) (*packages.Package, error) {
	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName, Dir: c.loadDir}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, err
	}

	if len(pkgs) < 1 {
		return nil, fmt.Errorf("package not found")
	}

	if len(pkgs[0].Errors) > 0 {
		return pkgs[0], pkgs[0].Errors[0]
	}

	return pkgs[0], nil
}
