# Decision: host documentation with Zensical

Status: the Zensical site, intent-based content split, strict build, and GitHub
Pages workflow are implemented and live at
[aleksandergreg.github.io/go-cli-tool](https://aleksandergreg.github.io/go-cli-tool/).
Deployment runs from `main` through GitHub Actions.

## Recommendation

OpsQuest uses one documentation site with clear audience routes, not separate player and engineering sites. The first deployment uses **Zensical on GitHub Pages**, a deliberately small Material-compatible `mkdocs.yml`, and a pinned generator version. This keeps the Markdown hierarchy as the content source, gives the site local search and structured navigation, and preserves Material for MkDocs as a practical fallback while Zensical matures.

Zensical is the forward-looking choice from the Material for MkDocs team and already supports the features OpsQuest needs: the MkDocs file layout, `README.md` index pages, navigation, strict link validation, Mermaid, built-in client-side search, and static output. The important caveat is that it is still alpha software with an evolving module API. The pilot should therefore avoid generator-specific customization, prove the exact build in CI, and make reverting to Material a configuration change rather than a content migration.

Use **Astro Starlight** instead if the priority changes from documentation-first to a highly branded game site with custom interactive components. Starlight is a stronger base for bespoke landing pages and future interactive mission previews, but it adds a Node/Astro content application and normally expects pages under `src/content/docs/`.

## Site information architecture

The site optimizes for three visitor questions: “How do I play?”, “What does
the game teach?”, and “How does it stay safe?”

```text
Home
├── Play
│   ├── Quick start
│   ├── Missions, hints, and progress
│   ├── Controls and teaching-shell commands
│   ├── Mission map
│   └── Docker Foundations
├── How OpsQuest works
│   ├── Learning philosophy
│   ├── Architecture
│   └── Sandbox and safety
├── Contribute
│   ├── Contributing and quality gates
│   ├── Mission authoring
│   └── Profiles and compatibility
└── Project
    ├── Roadmap
    ├── Changelog
    └── Releases
```

The public navigation does not mirror every repository directory. Iteration
reports, completed decisions, and infrastructure runbooks stay outside
`docs_dir`; current guidance and genuine proposals remain public. Top-level
sections are collapsible and use index pages so overview links do not become
duplicate sidebar entries.

Keep the root README concise and useful on package/repository pages: pitch, install, smallest quick start, safety summary, and links into the site. Move long-form controls, shell semantics, mission tables, and architecture explanations into the site rather than maintaining two copies.

## Platform comparison

| Option | Fit for OpsQuest | Main compromise | Verdict |
| --- | --- | --- | --- |
| [Zensical](https://zensical.org/docs/create-your-site/) | Uses the existing `docs/` Markdown layout, builds a static site, includes search, and supports MkDocs configuration | Still [alpha](https://zensical.org/about/roadmap/) and not yet at complete Material/plugin parity | Selected, with a pinned version and compatibility fallback |
| [Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) | Mature, excellent navigation/search, very small authoring burden, native Mermaid support | Python toolchain in a Go repository and a platform now transitioning toward Zensical | Conservative fallback and migration safety net |
| [Astro Starlight](https://starlight.astro.build/) | Accessible modern UI, built-in Pagefind search and i18n, Markdown/MDX, strong custom-page story | Adds Node/Astro structure and typically relocates content under `src/content/docs/` | Best alternative for a more branded or interactive site |
| [VitePress](https://vitepress.dev/guide/what-is-vitepress) | Fast, polished technical docs with Vue-enhanced Markdown | Vue/Node stack offers more programmability than this site currently needs | Good, but no decisive advantage over Starlight here |
| [Docusaurus](https://docusaurus.io/docs.html) | Strong React ecosystem, docs versioning, localization, and content features | Heavier application and maintenance model; versioning would be premature | Revisit only if multiple supported doc versions or a broader project website become real requirements |

Do not add documentation versioning initially. OpsQuest has one active product
line, and the repository-only delivery history already records milestones. Even
the Docusaurus guide warns that versioning duplicates content and adds
contributor and build complexity when it is not truly needed.

## Hosting and delivery

GitHub Pages is the natural first host because the source and release workflow already live on GitHub and every candidate produces static files. Use a custom Actions workflow with separate build and deploy jobs:

1. Pull requests run a pinned documentation build in strict mode and a link check, but never deploy.
2. Merges to `main` rebuild the same artifact and deploy it to the protected `github-pages` environment.
3. Start at the repository Pages URL. Add a custom `docs.*` domain only after the navigation and content stabilize.
4. Keep search local to the static artifact. Add analytics only when there is a concrete question to answer and a privacy decision has been made.

GitHub's [custom Pages workflow](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages) supports generator-built artifacts without requiring a separate hosting service.

## Content and maintenance rules

- Keep Markdown and editable diagram sources in this repository as the single source of truth.
- Give every public page `description`, `audience`, and `status` metadata.
- Keep current and proposed material visibly distinct; a shipped proposal must
  update a current guide and leave the roadmap.
- Keep historical reports and implemented decisions under `project/`, outside
  the public site source.
- Keep diagram source and SVG export together. The site should publish the SVG and link to the editable source on GitHub.
- Prefer stable relative links and fail the site build on broken internal links.
- Give every page one clear owner section. Cross-link rather than copy shared explanations.
- Review site claims in the same pull request as the behavior they describe.

## Rollout status

1. **Content foundation — complete:** public docs are organized by visitor
   intent; repository-only decisions and history have explicit homes.
2. **Generator proof of concept — complete:** Zensical is pinned, navigation
   remains Material-compatible, local preview and strict build targets exist,
   and no custom theme code is required.
3. **Player content pass — complete:** quick start, mission mechanics, controls, Linux worlds, and Docker Foundations have dedicated pages; the root README remains concise.
4. **Technical content pass — complete:** mission content, profile compatibility, and contribution workflows have focused pages alongside architecture and safety.
5. **Publish — complete:** pull requests build in strict mode; `main` builds, uploads, and deploys the GitHub Pages artifact. The first deployment completed successfully on July 22, 2026, and the public HTTPS endpoint returned the expected OpsQuest site.
6. **Reassess after use — future:** remain on Zensical if the pinned build is reliable; use the same content with Material if alpha churn is costly; consider Starlight only if custom interactive presentation becomes a product goal.

This structure keeps the information model independent from the renderer:
audience and lifecycle decide whether content is public, while Zensical only
presents the selected public source tree.
