# Go Proxy: Roteador Inteligente de APIs LLM

Este projeto implementa um proxy nativo em Go projetado para rotear e gerenciar requisições de APIs de Large Language Models (LLMs) de forma eficiente e resiliente. Ele atua como um intermediário entre suas aplicações e diversos provedores de LLMs, como Ollama, OpenRouter e Google Gemini, oferecendo conversão de formato, rotação automática de chaves de API e um sistema de fallback robusto.

## Funcionalidades Principais

-   **Proxy Nativo em Go**: Desenvolvido em Go para alta performance e baixo consumo de recursos.
-   **Compatibilidade com APIs Padrão**: Aceita requisições no formato da API OpenAI (`/v1/chat/completions`) e Anthropic (`/v1/messages`), convertendo-as para o formato específico de cada provedor de LLM.
-   **Suporte a Múltiplos Provedores**: Integração com:
    -   **Ollama**: Para modelos locais ou em nuvem (`:cloud`).
    -   **OpenRouter**: Para acesso a uma vasta gama de modelos (`:free`).
    -   **Google Gemini**: Para os modelos da Google (`gemini-*`, `palm`, `bison`).
-   **Rotação Automática de Chaves de API**: Gerencia múltiplas chaves por provedor, rotacionando-as automaticamente em caso de `rate limit` (código de status HTTP 429).
-   **Sistema de Fallback Configurable**: Em caso de esgotamento de chaves ou falha de um provedor, o proxy tenta automaticamente o próximo provedor na ordem de fallback definida.
-   **Carregamento de Configurações via `.env`**: Lê chaves de API e outras configurações de um arquivo `config.env`, facilitando a gestão de credenciais e o deploy.
-   **Logs Detalhados**: Fornece logs claros e informativos sobre o fluxo das requisições, tentativas de provedores, rotação de chaves e erros, essenciais para depuração e monitoramento.

## Como Funciona

O Go Proxy intercepta requisições HTTP em endpoints compatíveis com OpenAI e Anthropic. Com base no modelo especificado na requisição, ele detecta o provedor de destino (Ollama, OpenRouter ou Google). Em seguida, ele seleciona uma chave de API disponível para o provedor, converte o payload da requisição para o formato esperado pelo provedor e a encaminha. Se o provedor retornar um erro de `rate limit`, o proxy tenta a próxima chave. Se todas as chaves de um provedor falharem, ele tenta o próximo provedor na sequência de fallback configurada.

## Pré-requisitos

-   **Go 1.18 ou superior**: Instale a versão mais recente do Go em [golang.org/dl](https://golang.org/dl/).
-   **Chaves de API**: Obtenha as chaves de API para os provedores que você deseja utilizar (Ollama, OpenRouter, Google API).

## Configuração

Crie um arquivo chamado `config.env` na raiz do projeto com suas chaves de API e configurações. As variáveis de ambiente do sistema terão precedência sobre as definidas neste arquivo.

Exemplo de `config.env`:

```env
# ==============================
# SERVER
# ==============================
OLLAMA_PROXY_HOST=0.0.0.0
OLLAMA_PROXY_PORT=11436
OLLAMA_PROXY_TIMEOUT=300
OLLAMA_PROXY_RESET=3600

# ==============================
# OLLAMA CLOUD
# modelos devem terminar com :cloud
# ex: qwen3-coder:480b:cloud
# ==============================
OLLAMA_UPSTREAM=ollama.com
OLLAMA_API_KEYS=sua_chave_ollama_1,sua_chave_ollama_2

# ==============================
# OPENROUTER
# modelos devem terminar com :free
# ex: openai/gpt-oss-120b:free
# ==============================
OPENROUTER_UPSTREAM=openrouter.ai
OPENROUTER_API_KEYS=sua_chave_openrouter_1,sua_chave_openrouter_2

# ==============================
# GOOGLE GENERATIVE API
# modelos: gemini-3-flash-preview, gemini-2.5-flash-preview, palm, bison
# ==============================
GOOGLE_UPSTREAM=generativelanguage.googleapis.com
GOOGLE_API_KEYS=sua_chave_google_1,sua_chave_google_2

# ==============================
# ORDEM DE FALLBACK
# quando um provider esgota ou falha
# ==============================
PROVIDER_FALLBACK_ORDER=ollama,openrouter,google
```

## Como Compilar

1.  **Clone o repositório (ou descompacte o arquivo):**
    ```bash
    git clone https://github.com/seu-usuario/go-proxy.git # Se for um repositório
    cd go-proxy
    ```
    *(Se você recebeu o projeto como um arquivo zip, descompacte-o e navegue até a pasta raiz.)*

2.  **Compile para o seu sistema operacional atual:**
    ```bash
    go build -o proxy_go main.go
    ```
    Isso criará um executável chamado `proxy_go` (ou `proxy_go.exe` no Windows) na pasta atual.

3.  **Compilação Cruzada (Opcional - para outros sistemas):**
    Para compilar para um sistema diferente do seu:

    -   **Para Linux (64-bit):**
        ```bash
        GOOS=linux GOARCH=amd64 go build -o proxy_go_linux main.go
        ```
    -   **Para Windows (64-bit):**
        ```bash
        GOOS=windows GOARCH=amd64 go build -o proxy_go.exe main.go
        ```
    -   **Para macOS (Intel):**
        ```bash
        GOOS=darwin GOARCH=amd64 go build -o proxy_go_mac main.go
        ```
    -   **Para macOS (Apple Silicon/M1/M2):**
        ```bash
        GOOS=darwin GOARCH=arm64 go build -o proxy_go_mac_arm main.go
        ```

## Como Executar

1.  **Certifique-se de que seu `config.env` está na mesma pasta do executável `proxy_go`.
2.  **Execute o binário:**
    ```bash
    ./proxy_go
    ```
    O proxy iniciará e estará escutando em `http://0.0.0.0:11436` (ou na porta configurada em `OLLAMA_PROXY_PORT`).

## Exemplos de Uso com cURL

Você pode testar o proxy usando `curl` para enviar requisições para os diferentes provedores. Lembre-se de que o proxy escuta na porta `11436` por padrão.

### **Ollama (Exemplo de modelo `:cloud`)**

```bash
curl -s http://localhost:11436/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-next:80b:cloud","messages":[{"role":"user","content":"Diga olá."}],"stream":false}' \
  | python -m json.tool
```

### **Google Gemini (Exemplo de modelo `gemini-2.5-flash`)**

```bash
curl -s http://localhost:11436/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"Diga olá."}],"stream":false}' \
  | python -m json.tool
```

### **OpenRouter (Exemplo de modelo `:free`)**

```bash
curl -s http://localhost:11436/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/gpt-oss-120b:free","messages":[{"role":"user","content":"Diga olá."}],"stream":false}' \
  | python -m json.tool
```

## Logs e Depuração

O proxy gera logs detalhados no console, indicando o fluxo da requisição, o provedor detectado, a chave de API utilizada (parcialmente ofuscada por segurança), e quaisquer erros de upstream. Em caso de falha na comunicação com o provedor ou `rate limit`, o log fornecerá informações cruciais para depuração, incluindo o corpo da resposta de erro do provedor, se disponível.

## Estrutura do Projeto

```
go-proxy/
├── anthropic/             # Lógica de conversão para API Anthropic
│   └── anthropic.go
├── api/                   # Definições de tipos internos e compatíveis com Ollama
│   └── types.go
├── google/                # Lógica de conversão para Google Gemini API
│   └── google.go
├── openai/                # Definições de tipos compatíveis com OpenAI API
│   └── openai.go
├── internal/              # Utilitários internos (ex: orderedmap)
│   └── orderedmap/
│       └── orderedmap.go
├── go.mod                 # Módulo Go e dependências
├── main.go                # Lógica principal do proxy, roteamento e handlers
├── config.env.example     # Exemplo de arquivo de configuração .env
└── README.md              # Este arquivo
```

## Contribuição

Contribuições são bem-vindas! Sinta-se à vontade para abrir issues ou pull requests para melhorias, correções de bugs ou novas funcionalidades.

## Licença

Este projeto está licenciado sob a licença MIT. Veja o arquivo `LICENSE` para mais detalhes. (Se aplicável)
