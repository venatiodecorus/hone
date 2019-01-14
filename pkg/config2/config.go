package config

import (
	"os"
	"strings"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/hashicorp/hcl2/gohcl"
	"github.com/justinbarrick/hone/pkg/git"
	"github.com/justinbarrick/hone/pkg/job"
	"github.com/justinbarrick/hone/pkg/logger"
	"github.com/justinbarrick/hone/pkg/graph"
	"github.com/justinbarrick/hone/pkg/graph/node"
	"github.com/justinbarrick/hone/pkg/secrets/vault"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

type Remains interface {
	GetRemain() hcl.Body
}

type Parser struct {
	parser  *hclparse.Parser
	body    hcl.Body
	remain  hcl.Body
	ctx *hcl.EvalContext
}

func NewParser() Parser {
	return Parser{
		parser: hclparse.NewParser(),
	}
}

func (p *Parser) Parse(config string) error {
	hclFile, diags := p.parser.ParseHCL([]byte(config), "test")
	p.remain = hclFile.Body
	p.body = hclFile.Body
	return p.checkErrors(diags)
}

func (p *Parser) checkErrors(err error) error {
	switch e := err.(type) {
	case hcl.Diagnostics:
		if e.HasErrors() {
			wr := hcl.NewDiagnosticTextWriter(os.Stderr, p.parser.Files(), 78, true)
			wr.WriteDiagnostics(e)
			return e
		}
		return nil
	}
	return err
}

func (p *Parser) GetContext() *hcl.EvalContext {
	if p.ctx == nil {
		p.ctx = &hcl.EvalContext{}
	}

	if p.ctx.Functions == nil {
		p.ctx.Functions = map[string]function.Function{}
		p.ctx.Functions["not"] = stdlib.NotFunc
		p.ctx.Functions["and"] = stdlib.AndFunc
		p.ctx.Functions["or"] = stdlib.OrFunc
		p.ctx.Functions["bytesLen"] = stdlib.BytesLenFunc
		p.ctx.Functions["bytesSlice"] = stdlib.BytesSliceFunc
		p.ctx.Functions["hasIndex"] = stdlib.HasIndexFunc
		p.ctx.Functions["index"] = stdlib.IndexFunc
		p.ctx.Functions["length"] = stdlib.LengthFunc
		p.ctx.Functions["csvDecode"] = stdlib.CSVDecodeFunc
		p.ctx.Functions["formatDate"] = stdlib.FormatDateFunc
		p.ctx.Functions["format"] = stdlib.FormatFunc
		p.ctx.Functions["formatList"] = stdlib.FormatListFunc
		p.ctx.Functions["equal"] = stdlib.EqualFunc
		p.ctx.Functions["notEqual"] = stdlib.NotEqualFunc
		p.ctx.Functions["coalesce"] = stdlib.CoalesceFunc
		p.ctx.Functions["jsonEncode"] = stdlib.JSONEncodeFunc
		p.ctx.Functions["jsonDecode"] = stdlib.JSONDecodeFunc
		p.ctx.Functions["absolute"] = stdlib.AbsoluteFunc
		p.ctx.Functions["add"] = stdlib.AddFunc
		p.ctx.Functions["subtract"] = stdlib.SubtractFunc
		p.ctx.Functions["multiply"] = stdlib.MultiplyFunc
		p.ctx.Functions["divide"] = stdlib.DivideFunc
		p.ctx.Functions["modulo"] = stdlib.ModuloFunc
		p.ctx.Functions["greaterThan"] = stdlib.GreaterThanFunc
		p.ctx.Functions["greaterThanOrEqualTo"] = stdlib.GreaterThanOrEqualToFunc
		p.ctx.Functions["lessThan"] = stdlib.LessThanFunc
		p.ctx.Functions["lessThanOrEqualTo"] = stdlib.LessThanOrEqualToFunc
		p.ctx.Functions["negate"] = stdlib.NegateFunc
		p.ctx.Functions["min"] = stdlib.MinFunc
		p.ctx.Functions["max"] = stdlib.MaxFunc
		p.ctx.Functions["int"] = stdlib.IntFunc
		p.ctx.Functions["concat"] = stdlib.ConcatFunc
		p.ctx.Functions["hasElement"] = stdlib.SetHasElementFunc
		p.ctx.Functions["union"] = stdlib.SetUnionFunc
		p.ctx.Functions["intersection"] = stdlib.SetIntersectionFunc
		p.ctx.Functions["setSubtract"] = stdlib.SetSubtractFunc
		p.ctx.Functions["diff"] = stdlib.SetSymmetricDifferenceFunc
		p.ctx.Functions["upper"] = stdlib.UpperFunc
		p.ctx.Functions["lower"] = stdlib.LowerFunc
		p.ctx.Functions["reverse"] = stdlib.ReverseFunc
		p.ctx.Functions["strlen"] = stdlib.StrlenFunc
		p.ctx.Functions["substr"] = stdlib.SubstrFunc
	}

	if p.ctx.Variables == nil {
		p.ctx.Variables = map[string]cty.Value{}
	}

	return p.ctx
}

func (p *Parser) Decode(body hcl.Body, val interface{}) error {
	return gohcl.DecodeBody(body, p.GetContext(), val)
}

func (p *Parser) DecodeRemains(val Remains) error {
	err := p.checkErrors(p.Decode(p.remain, val))

	p.remain = val.GetRemain()

	return err
}

func (p *Parser) DecodeBody(val interface{}) error {
	return p.checkErrors(p.Decode(p.body, val))
}

func (p *Parser) DecodeEnv() (map[string]string, error) {
	envMap := map[string]string{}

	envStruct := struct {
		Env *[]string `hcl:"env"`
		Remain hcl.Body `hcl:",remain"`
	}{}

	err := p.DecodeBody(&envStruct)
	if err != nil {
		return envMap, err
	}

	if envStruct.Env != nil {
		for _, key := range *envStruct.Env {
			env := strings.SplitN(key, "=", 2)
			defaultVal := ""
			if len(env) > 1 {
				defaultVal = env[1]
			}
			val := os.Getenv(env[0])
			if val == "" {
				val = defaultVal
			}
			envMap[env[0]] = val
		}
	}

	if repo, err := git.NewRepository(); err == nil {
		for key, value := range repo.GitEnv() {
			envMap[key] = value
		}
	} else {
		logger.Printf("Failed to load git environment: %s", err)
	}

	p.ctx.Variables["env"], err = gocty.ToCtyValue(envMap, cty.Map(cty.String))
	if err != nil {
		return envMap, err
	}

	return envMap, nil
}

func (p *Parser) DecodeSecrets() (map[string]string, error) {
	secretsMap := map[string]string{}

	setSecrets := func() (err error) {
		if len(secretsMap) == 0 {
			p.ctx.Variables["secrets"] = cty.MapValEmpty(cty.String)
			return
		}

		p.ctx.Variables["secrets"], err = gocty.ToCtyValue(secretsMap, cty.Map(cty.String))
		return
	}

	secretsStruct := struct {
		Secrets   *[]string    `hcl:"secrets"`
		Workspace *string      `hcl:"workspace"`
		Vault     *vault.Vault `hcl:"vault,block"`
		Remain    hcl.Body     `hcl:",remain"`
	}{}

	err := p.DecodeBody(&secretsStruct)
	if err != nil {
		return secretsMap, err
	}

	if secretsStruct.Secrets == nil {
		return secretsMap, setSecrets()
	}

	workspace := "default"
	if secretsStruct.Workspace != nil {
		workspace = *secretsStruct.Workspace
	}

	secrets := []string{}
	if secretsStruct.Secrets != nil {
		secrets = *secretsStruct.Secrets
	}

	for _, secret := range secrets {
		secretsMap[secret] = os.Getenv(secret)
	}

	if secretsStruct.Vault == nil || secretsStruct.Vault.Token == "" {
		return secretsMap, setSecrets()
	}

	err = secretsStruct.Vault.Init()
	if err != nil {
		return secretsMap, err
	}

	secretsMap, err = secretsStruct.Vault.LoadSecrets(workspace, secrets)
	if err != nil {
		return secretsMap, err
	}

	return secretsMap, setSecrets()
}

func (p *Parser) DecodeJobs() ([]*job.Job, error) {
	load := struct {
		Jobs []struct {
			Name string `hcl:"name,label"`
			Remain hcl.Body `hcl:",remain"`
		} `hcl:"job,block"`
		Remain hcl.Body `hcl:",remain"`
	}{}

	if err := p.DecodeBody(&load); err != nil {
		return nil, err
	}

	g := graph.NewGraph(nil)

	remains := map[string]hcl.Body{}

	for _, partialJob := range load.Jobs {
		remains[partialJob.Name] = partialJob.Remain

		j := &job.Job{
			Name: partialJob.Name,
		}

		g.AddNode(j)

		attributes, diags := partialJob.Remain.JustAttributes()
		if err := p.checkErrors(diags); err != nil {
			return nil, err
		}

		for _, attr := range attributes {
			variables := attr.Expr.Variables()
			for _, variable := range variables {
				if variable.RootName() != "jobs" {
					continue
				}

				depName, ok := variable[1].(hcl.TraverseAttr)
				if ! ok {
					continue
				}

				g.AddDep(j, depName.Name)
			}
		}
	}

	jobs := []*job.Job{}

	errors := g.IterSorted(func(node node.Node) (err error) {
		job := node.(*job.Job)

		if err := p.decodeJob(job, remains[job.GetName()], 0); err != nil {
			return err
		}

		jobs = append(jobs, job)
		return nil
	})
	if len(errors) > 0 {
		return nil, errors[0]
	}

	return jobs, nil
}

func (p *Parser) decodeJob(job *job.Job, body hcl.Body, depth int) error {
	decodeErr := p.Decode(body, job)

	jobMap := map[string]cty.Value{}
	if ! p.ctx.Variables["jobs"].IsNull() {
		jobMap = p.ctx.Variables["jobs"].AsValueMap()
	}

	jobCty, err := job.ToCty()
	if err != nil {
		return err
	}

	jobMap[job.Name] = jobCty
	p.ctx.Variables["jobs"] = cty.MapVal(jobMap)

	switch e := decodeErr.(type) {
	case hcl.Diagnostics:
		if depth > 20 {
			return p.checkErrors(e)
		} else {
			return p.decodeJob(job, body, depth + 1)
		}
	default:
		return err
	}

	return nil
}
