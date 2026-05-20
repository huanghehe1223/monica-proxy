---
name: edit-figure
description: Convert a scientific, technical, or diagrammatic image into an editable draw.io or SVG file using the Edit-Figure-MCP tools. Use when the user asks to convert an image, figure, research diagram, workflow diagram, paper illustration, architecture diagram, or UI-like figure into editable draw.io/diagrams.net XML or SVG; default to draw.io when the target format is not specified.
---

# Edit Figure

Use this skill to convert a raster figure into an editable draw.io or SVG file. The MCP server handles segmentation, icon cropping, background removal, and placeholder replacement. The model handles visual reasoning and writes the editable template file into the workspace.

## Skill Project Layout

File structure:

```text
edit-figure/
|-- SKILL.md
|-- agents/
|   `-- openai.yaml
`-- prompts/
    |-- icon_discovery.md
    |-- sam3_prompt_selector.md
    |-- template_prompt_drawio.md
    `-- template_prompt_svg.md
```

- `SKILL.md`: main workflow and routing instructions.
- `agents/openai.yaml`: UI-facing skill metadata.
- `prompts/icon_discovery.md`: freely discover visible icon-like or semantic visual elements.
- `prompts/sam3_prompt_selector.md`: select suitable discovered elements and convert them into the final `sam3_prompt`.
- `prompts/template_prompt_drawio.md`: guidance for writing `{output_dir}/template.drawio`.
- `prompts/template_prompt_svg.md`: guidance for writing `{output_dir}/template.svg`.
## Critical Read Requirement

This skill only works correctly when the model has read the complete relevant files for the current task.

Before executing any workflow step, the model must read this `SKILL.md` completely from beginning to end.

For any workflow step that depends on another file, the model must also read that required file completely before using it. This includes, but is not limited to:

- referenced prompt/resource files under `prompts/`
- tool-returned structured files such as `boxlib.json`
- any template, configuration, manifest, metadata, or intermediate file that is used to generate the final editable output

Do not proceed after reading only a partial line range, partial chunk, truncated preview, or summarized view of any required file. A partial read may omit critical constraints, object records, coordinates, placeholder IDs, or layout information, and can lead to severely incorrect output.

If the file reader returns bounded ranges, such as lines 1-50 or lines 51-150, continue reading subsequent ranges until the end of the file is reached.

For structured files such as JSON, read and parse the complete file content before relying on its data. Do not infer missing objects, coordinates, labels, or placeholders from a partial preview.

The model must not call downstream tools, generate templates, replace placeholders, or produce final outputs unless all required files for that step have been read completely.

If the model cannot confirm that `SKILL.md` and all required files for the current step have been read completely, it must stop and report that the required skill files or task files were only partially loaded.

## Resources

Read only the prompt needed for the current step, but always read that selected prompt completely through its EOF marker:

- [icon discovery prompt](./prompts/icon_discovery.md): freely identify visible icon-like, object-like, semantic, or visually meaningful elements in the original image.
- [SAM3 prompt selector](./prompts/sam3_prompt_selector.md): choose suitable elements from the discovery result and convert them into the final `sam3_prompt`.
- [draw.io template prompt](./prompts/template_prompt_drawio.md): create `{output_dir}/template.drawio`.
- [SVG template prompt](./prompts/template_prompt_svg.md): create `{output_dir}/template.svg`.

## Critical Resource Read Contract

When reading any prompt/resource file for this skill, read it from line 1 through EOF before using it.

If the file reader returns a bounded range such as lines 1-50 or lines 51-150, continue reading the next ranges until the explicit EOF marker is reached.

Do not proceed based on a partial read.

Required EOF markers:
- `prompts/icon_discovery.md` must end with `<!-- END_OF_FILE: icon_discovery.md -->`
- `prompts/sam3_prompt_selector.md` must end with `<!-- END_OF_FILE: sam3_prompt_selector.md -->`
- `prompts/template_prompt_drawio.md` must end with `<!-- END_OF_FILE: template_prompt_drawio.md -->`
- `prompts/template_prompt_svg.md` must end with `<!-- END_OF_FILE: template_prompt_svg.md -->`

If an EOF marker is not found, stop and report that the resource file was not fully read.


## Workflow

1. Determine the target format.
   - If the user explicitly asks for SVG, produce SVG.
   - Otherwise default to draw.io.

2. Plan segmentation.
   - Read `prompts/icon_discovery.md` completely.
   - Inspect the original image.
   - Follow `prompts/icon_discovery.md` to freely identify visible icon-like, object-like, semantic, or visually meaningful elements.
   - Respond in chat with the discovery result from `prompts/icon_discovery.md`.
   - Read `prompts/sam3_prompt_selector.md` completely.
   - From the previously discovered elements, follow `prompts/sam3_prompt_selector.md` to choose suitable elements for detection.
   - Convert selected elements into broad, simple English nouns or short noun phrases.
   - Use the selector prompt's final `Tool argument` as the exact `sam3_prompt` string.

3. Call MCP tool `segment_and_extract_icons`.
   - Required arguments:
     - `workspace_dir`: absolute workspace path.
     - `image_path`: image path relative to `workspace_dir`.
     - `sam3_prompt`: comma-separated prompt string from step 2.
   - Use the returned `output_dir`, `samed_image`, `boxlib_json`, and `icon_infos_json`.
   - `icon_infos.json` is the detailed object record; avoid large per-object chat output.

4. Review segmentation quality once.
   - Open and inspect `samed_image` (`samed.png`) after the first `segment_and_extract_icons` call.
   - Compare it with the original image and check whether semantic visual objects that should be preserved as images were missed or left unmasked.
   - If the missed objects can be fixed by adding suitable concepts to `sam3_prompt`, call `segment_and_extract_icons` one more time at most.
   - The second call's `sam3_prompt` must be complete: include every prompt from the first call plus the added prompts. Do not pass only the newly added prompts.
   - Reuse the second call's returned outputs for the rest of the workflow.
   - If the first result is acceptable, or if the misses are editable elements that should be redrawn, do not rerun segmentation.

5. Generate the editable template file in the MCP output directory.
   - For draw.io:
     - Read `prompts/template_prompt_drawio.md` completely before writing the template.
     - Strictly follow all requirements, constraints, formatting rules, placeholder rules, output rules, and validation instructions in `prompts/template_prompt_drawio.md`.
     - Write directly to `{output_dir}/template.drawio`.
     - Set `template_path` to that relative path.
   - For SVG:
     - Read `prompts/template_prompt_svg.md` completely before writing the template.
     - Strictly follow all requirements, constraints, formatting rules, placeholder rules, output rules, and validation instructions in `prompts/template_prompt_svg.md`.
     - Write directly to `{output_dir}/template.svg`.
     - Set `template_path` to that relative path.
   - Treat the selected template prompt file as the authoritative specification for generating the template.
   - Do not simplify, reinterpret, skip, or partially apply the selected template prompt's instructions.
   - Use the original image, `samed_image`, and the complete `boxlib_json` as required inputs.
   - Read and parse the complete `boxlib_json` before writing the template.
   - Preserve the original image coordinate system from `boxlib.json.image_size`.
   - Create placeholders only as required by the selected template prompt and `boxlib.json`.
   - Do not embed the original raster figure or cropped icons in the template unless the selected template prompt explicitly allows it.
   - Do not paste the full template code in chat unless the user explicitly asks; create the file in the workspace.

6. Preview and optimize the template.
   - After writing `template.drawio` or `template.svg`, call MCP tool `export_diagram_png` with `workspace_dir` and `source_path` set to `{output_dir}/template.drawio` or `{output_dir}/template.svg`, relative to `workspace_dir`.
   - Open and inspect the exported template PNG; compare it against both the original input image and `samed_image` (`samed.png`).
   - Do not assume `.drawio` or `.svg` files can be visually inspected directly; they must be exported to PNG before visual comparison.
   - Check whether model-drawn editable non-icon elements are correctly positioned relative to the masked/icon placeholder regions in `samed_image`; if misaligned, adjust editable elements first instead of moving/resizing masked/icon placeholder regions.
   - Only adjust placeholder positions or sizes when the preview clearly shows that the placeholder geometry from `boxlib.json` is wrong or unusable.
   - Check whether all model-drawn editable non-icon elements are faithfully reconstructed from the original image, including basic shapes, lines/arrows, text, labels, containers, connectors, colors, approximate stroke widths, spacing/alignment, and layering order.
   - If any non-icon basic element is missing, oversimplified, poorly shaped, poorly aligned, incorrectly layered, or visually inconsistent with the original image, revise and improve the same `template.*` file.
   - Prefer improving editable vector/text elements over changing icon placeholders.
   - Preserve every required placeholder ID from `boxlib.json`; do not invent, remove, rename, or duplicate `<AF>xx` placeholders during optimization.
   - Do not embed the original raster figure or cropped icons during optimization.
   - After each revision, call `export_diagram_png` again on the revised `template.*`, then open and inspect the new exported PNG before deciding whether another revision is needed.
   - Run at most 3 template optimization passes; stop early when there are no major visual issues.
   - Minor unavoidable differences are acceptable if the editable structure is correct and further changes are unlikely to improve the result.

7. Call MCP tool `replace_template_placeholders`.
   - Required arguments:
     - `workspace_dir`: same absolute workspace path.
     - `template_path`: `{output_dir}/template.drawio` or `{output_dir}/template.svg`, relative to `workspace_dir`.
   - Leave `output_format` as `auto` unless the suffix is ambiguous.
   - Because the template is written beside `icon_infos.json`, `icon_infos_relative_path` is usually unnecessary.
   - The tool writes `final.drawio` or `final.svg` beside the template.

## Output Discipline

- Return the final relative path from the replacement tool.
- Mention the generated `template.*`, `template preview PNG`, `final.*` paths.
- If segmentation finds no icons, still generate the editable template and run replacement.
- If producing both formats, reuse the same segmentation output and run template generation plus replacement once per format.

## Placeholder Rules

- Placeholder IDs must match `boxlib.json` labels after removing angle brackets: `<AF>01` becomes `AF01`.
- The replacement tool depends on those IDs and on `icon_infos.json`.
- Do not invent extra AF placeholders.
- Do not omit AF placeholders found in `boxlib.json`.
- Use draw.io as the default target because it is easier to edit in diagrams.net.
