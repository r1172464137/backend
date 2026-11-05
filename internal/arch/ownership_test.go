package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// repositoryOwners maps every shared repository contract to the module that
// owns its tables (ADR-001, step 5). Only the owner uses it; other modules go
// through the owner's facade (rules 2 and 4). Contracts not listed belong to
// the platform kernel, which every module may use.
var repositoryOwners = map[string]string{
	"UserRepo": "identity", "UserAuthRepo": "identity", "UserDeviceRepo": "identity", "AuthRepo": "identity",
	"UserCacheRepo": "identity", "IdentityStore": "identity", "IdentityTransactor": "identity",

	"OrderRepo": "billing", "OrderEventRepo": "billing", "PaymentRepo": "billing", "CouponRepo": "billing",
	"UserWithdrawalRepo": "billing", "WalletRepo": "billing", "BillingStore": "billing", "BillingTransactor": "billing",

	"SubscribeRepo": "subscription", "UserSubscriptionRepo": "subscription", "SubscriptionTrafficRepo": "subscription",
	"EntitlementRepo": "subscription", "ClientRepo": "subscription", "SubscriptionStore": "subscription",
	"SubscriptionTransactor": "subscription",

	"NodeRepo": "network", "TrafficRepo": "network", "NetworkStore": "network", "NetworkTransactor": "network",

	"TicketRepo": "support", "AnnouncementRepo": "support", "AdsRepo": "support", "DocumentRepo": "support",

	"TelegramTopicRepo": "notification",
}

// transactionOwners maps each scoped transaction to the module that owns it.
var transactionOwners = map[string]string{
	"InIdentityTx": "identity", "InBillingTx": "billing", "InSubscriptionTx": "subscription", "InNetworkTx": "network",
}

// A module reads and writes another module's data only through that module's
// facade: using its repository contract or opening its transaction couples
// the two at the persistence layer, which the service split cannot undo.
func TestModulesUseOnlyTheirOwnRepositories(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	var violations []string
	err := filepath.WalkDir(filepath.Join(root, "internal", "module"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		module := strings.Split(strings.TrimPrefix(rel, "internal/module/"), "/")[0]
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		repository := importName(file, importPrefix+"internal/repository")
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && repository != "" && id.Name == repository {
					if owner, ok := repositoryOwners[n.Sel.Name]; ok && owner != module {
						violations = append(violations, rel+": uses "+owner+"'s repository."+n.Sel.Name)
					}
				}
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
					if owner, ok := transactionOwners[sel.Sel.Name]; ok && owner != module {
						violations = append(violations, rel+": opens "+owner+"'s "+sel.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk modules: %v", err)
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("%s — call the owner's facade instead (docs/design/adr-001-modular-monolith.md, rules 2 and 4)", v)
	}
}

// The entry points (HTTP middleware, task handlers, the CLI and the runtime
// bootstrap) decode a request and call a module; they do not read or write a
// module's tables themselves. The composition root hands each module its own
// repositories and nothing else: an accessor of a module-owned repository may
// only appear inside that module's constructor call.
func TestEntryPointsReachModuleDataThroughModules(t *testing.T) {
	root := repoRoot(t)
	accessors := storeAccessorOwners(t, root)
	fset := token.NewFileSet()
	var violations []string
	for _, dir := range []string{"cmd", "internal/app", "internal/transport"} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			violations = append(violations, repositoryReads(fset, file, filepath.ToSlash(rel), accessors)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("%s — call the owning module's facade instead (docs/design/adr-001-modular-monolith.md, rules 2 and 4)", v)
	}
}

// repositoryReads lists the calls of file (at rel) to a module-owned store
// accessor that the entry-point rule forbids. In the composition root's own
// package an accessor may appear inside its owner's constructor call
// (<owner>.New...), which hands the module its own repositories.
func repositoryReads(fset *token.FileSet, file *ast.File, rel string, accessors map[string]string) []string {
	compositionRoot := path.Dir(rel) == "internal/app"
	var violations []string
	var constructors []string // owners of the enclosing module constructor calls
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if owner, ok := accessors[sel.Sel.Name]; ok && len(call.Args) == 0 && (!compositionRoot || !slices.Contains(constructors, owner)) {
			violations = append(violations, fmt.Sprintf("%s:%d: reads %s's repository through %s()", rel, fset.Position(call.Pos()).Line, owner, sel.Sel.Name))
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || !strings.HasPrefix(sel.Sel.Name, "New") {
			return true
		}
		constructors = append(constructors, pkg.Name)
		ast.Inspect(sel, visit)
		for _, arg := range call.Args {
			ast.Inspect(arg, visit)
		}
		constructors = constructors[:len(constructors)-1]
		return false
	}
	ast.Inspect(file, visit)
	return violations
}

// storeAccessorOwners maps each repository accessor of the shared store to
// the module owning the repository it returns; kernel repositories are left
// out.
func storeAccessorOwners(t *testing.T, root string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal", "repository", "store.go"), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse the store: %v", err)
	}
	owners := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Params.List) != 0 {
			continue
		}
		if result, ok := fn.Type.Results.List[0].Type.(*ast.Ident); ok {
			if owner, ok := repositoryOwners[result.Name]; ok {
				owners[fn.Name.Name] = owner
			}
		}
	}
	if len(owners) == 0 {
		t.Fatal("found no module-owned store accessors in internal/repository/store.go")
	}
	return owners
}

// An integration event's topic is a constant its producer exports (for
// identity.user_registered, identity.UserRegisteredTopic): a subscriber
// spelling the topic, or a producer appending under a literal, drifts from the
// other side without any compiler or test noticing.
func TestEventTopicsAreNamedConstants(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			var topic ast.Expr
			switch {
			case sel.Sel.Name == "Subscribe" && len(call.Args) == 3: // bus.Subscribe(topic, consumer, handler)
				topic = call.Args[0]
			case sel.Sel.Name == "Append" && len(call.Args) == 4: // outbox.Append(ctx, topic, key, payload)
				topic = call.Args[1]
			default:
				return true
			}
			if lit, ok := topic.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				t.Errorf("%s:%d: event topic %s is spelled out; use the producer's exported constant", filepath.ToSlash(rel), fset.Position(lit.Pos()).Line, lit.Value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
}

// platform is the shared kernel every module may depend on, so it depends on
// no module itself — neither directly nor through the shared repository
// package, which references every module's entities.
func TestPlatformDependsOnNoModule(t *testing.T) {
	for _, f := range collectGoFiles(t) {
		if !within(f.dir, "internal/module/platform") || strings.HasSuffix(f.path, "_test.go") {
			continue
		}
		for _, imp := range f.imports {
			if within(imp, "internal/module") && !within(imp, "internal/module/platform") {
				t.Errorf("%s: the platform kernel imports module package %q", f.path, imp)
			}
			if imp == "internal/repository" {
				t.Errorf("%s: the platform kernel imports internal/repository, which references every module; use the kernel contracts", f.path)
			}
		}
	}
}

// Task handlers decode a message and call a module; the module owns the
// transaction. A handler opening one takes a module's rules out of the
// module.
func TestTaskHandlersOpenNoTransactions(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, "internal", "transport", "task"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "In") && strings.HasSuffix(sel.Sel.Name, "Tx") {
				t.Errorf("%s: a task handler opens %s; move the work into the owning module", filepath.ToSlash(rel), sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk task handlers: %v", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// importName is the name a file uses for the package at path, or "" when it
// does not import it.
func importName(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if value, _ := strconv.Unquote(spec.Path.Value); value == path {
			if spec.Name != nil {
				return spec.Name.Name
			}
			return path[strings.LastIndex(path, "/")+1:]
		}
	}
	return ""
}
