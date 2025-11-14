# Taskfile Architecture

This document explains the Taskfile structure and how it's organized for testing with k3d.

## Global Variables

```yaml
vars:
  IMAGE: 'efortin/vllm-chill'
  TAG: 'latest'
  CLUSTER_NAME: vllm-test
  K8S_CONTEXT: k3d-vllm-test
  NAMESPACE: vllm
```

These variables are used throughout all tasks to ensure consistency.

## Task Organization

### k3d Cluster Management (Subtasks)

The k3d setup is broken into modular subtasks:

#### 1. `k3d:install` (internal)
- **Purpose**: Install k3d if not already present
- **Status Check**: `command -v k3d` (skips if already installed)
- **Usage**: Automatically called as dependency

#### 2. `k3d:create` (internal)
- **Purpose**: Create the k3d cluster
- **Dependencies**: `k3d:install`
- **Status Check**: Skips if cluster already exists
- **Context**: Always uses `{{.K8S_CONTEXT}}` for kubectl commands
- **Actions**:
  - Creates cluster with 1 agent node
  - Verifies cluster info with explicit context
  - Gets nodes with explicit context

#### 3. `k3d:namespace` (internal)
- **Purpose**: Create the vllm namespace
- **Context**: Uses `{{.K8S_CONTEXT}}` for all kubectl commands
- **Actions**:
  - Creates namespace using `kubectl --context`
  - Uses `--dry-run=client` for idempotency

#### 4. `k3d:crd` (internal)
- **Purpose**: Install VLLMModel CRD and wait for it to be ready
- **Context**: Uses `{{.K8S_CONTEXT}}` for all kubectl commands
- **Actions**:
  - Applies CRD manifest with context
  - Waits for CRD to be established (retries up to 30 times)
  - Verifies CRD existence with context

#### 5. `k3d:models` (internal)
- **Purpose**: Create test VLLMModel resources
- **Context**: Uses `{{.K8S_CONTEXT}}` for all kubectl commands
- **Actions**:
  - Applies qwen3-coder model
  - Applies deepseek-r1 model
  - Lists all models

#### 6. `k3d:setup` (public)
- **Purpose**: Orchestrate all k3d setup subtasks
- **Actions**: Calls subtasks in order:
  1. `k3d:create`
  2. `k3d:namespace`
  3. `k3d:crd`
  4. `k3d:models`

#### 7. `k3d:teardown` (public)
- **Purpose**: Delete the k3d cluster
- **Actions**: Deletes cluster (ignores errors if not found)

### Test Tasks

#### `test:integration`
- **Description**: Run integration tests
- **Dependencies**: `k3d:setup` (ensures cluster is ready)
- **Environment**: Sets `KUBECONFIG`
- **Actions**:
  1. Switches to k3d context
  2. Runs integration tests with `-tags=integration`
  3. 5-minute timeout

#### `test:integration:coverage`
- **Description**: Run integration tests with coverage report
- **Dependencies**: `k3d:setup`
- **Environment**: Sets `KUBECONFIG`
- **Actions**:
  1. Switches to k3d context
  2. Runs tests with coverage
  3. Generates HTML report
  4. Shows coverage summary

#### `test:all`
- **Description**: Run all tests (unit + integration)
- **Actions**:
  1. Runs `test` (unit tests)
  2. Runs `test:integration`

#### `test:coverage:all`
- **Description**: Combined coverage report
- **Dependencies**: `k3d:setup`
- **Environment**: Sets `KUBECONFIG`
- **Actions**:
  1. Runs unit tests with coverage
  2. Runs integration tests with coverage
  3. Merges both coverage reports
  4. Generates combined HTML report

## Context Enforcement

**All kubectl commands use `--context {{.K8S_CONTEXT}}`** to ensure:
- Commands never run against wrong cluster
- Safe for users with multiple contexts
- Explicit context in CI/CD environments
- Prevents accidental production cluster access

Example:
```bash
kubectl apply --context k3d-vllm-test -f manifest.yaml
kubectl get pods --context k3d-vllm-test -n vllm
```

## Idempotency

Tasks are designed to be idempotent:

- **k3d:install**: Uses `status` to skip if k3d exists
- **k3d:create**: Uses `status` to skip if cluster exists
- **k3d:namespace**: Uses `--dry-run=client` to avoid errors
- **k3d:crd**: Retries CRD establishment check
- **k3d:models**: kubectl apply is idempotent by nature

Running `task k3d:setup` multiple times is safe.

## Internal vs Public Tasks

### Internal Tasks
- Marked with `internal: true`
- Not shown in `task --list`
- Used as building blocks

Internal tasks:
- `k3d:install`
- `k3d:create`
- `k3d:namespace`
- `k3d:crd`
- `k3d:models`

### Public Tasks
- Shown in `task --list`
- Intended for direct use

Public tasks:
- `k3d:setup`
- `k3d:teardown`
- `test:integration`
- `test:integration:coverage`
- `test:all`
- `test:coverage:all`

## Dependencies

Task dependencies are declarative:

```yaml
"test:integration":
  deps: [k3d:setup]  # Ensures cluster is ready before tests
```

Task automatically runs dependencies first.

## Usage Examples

### Setup k3d cluster
```bash
task k3d:setup
```

This will:
1. Install k3d (if needed)
2. Create cluster (if needed)
3. Create namespace
4. Install CRD
5. Create test models

### Run integration tests
```bash
task test:integration
```

Automatically sets up k3d cluster first.

### Run all tests with coverage
```bash
task test:coverage:all
```

Generates `combined-coverage.html` with both unit and integration coverage.

### Clean up
```bash
task k3d:teardown
```

## CI/CD Integration

In GitHub Actions:

```yaml
- name: Setup Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.23'

- name: Install Task
  run: |
    sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

- name: Setup k3d cluster
  run: task k3d:setup

- name: Run integration tests
  run: task test:integration
```

The workflow uses the same Taskfile as local development.

## Benefits of This Architecture

1. **Modularity**: Each subtask has a single responsibility
2. **Reusability**: Subtasks can be composed differently
3. **Safety**: Explicit context prevents accidents
4. **Idempotency**: Can run repeatedly without issues
5. **Consistency**: Same tasks work locally and in CI
6. **Maintainability**: Easy to modify individual steps
7. **Visibility**: Clear separation of public vs internal tasks

## Environment Variables

### Set by Taskfile
- `KUBECONFIG`: Set to `~/.kube/config` for test tasks

### Used by Taskfile
- `IMAGE`: Docker image name (default: `efortin/vllm-chill`)
- `TAG`: Docker image tag (default: `latest`)
- `HOME`: User home directory (for KUBECONFIG path)

## Troubleshooting

### Cluster already exists
Task will detect and skip creation. To recreate:
```bash
task k3d:teardown
task k3d:setup
```

### Wrong context
All commands use explicit context, so this shouldn't happen. If kubectl commands fail, verify:
```bash
kubectl config get-contexts
kubectl config use-context k3d-vllm-test
```

### CRD not ready
The `k3d:crd` task retries up to 30 times. If it still fails:
```bash
kubectl get crd models.vllm.sir-alfred.io --context k3d-vllm-test
kubectl describe crd models.vllm.sir-alfred.io --context k3d-vllm-test
```

## Future Enhancements

Possible improvements:
1. Add `k3d:status` task to check cluster health
2. Add `k3d:logs` task to view k3d logs
3. Add `k3d:port-forward` for local testing
4. Add `test:integration:watch` for TDD workflow
5. Add `k3d:reset` to recreate cluster from scratch
