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
    |-- sam3_prompt_generator.md
    |-- template_prompt_drawio.md
    `-- template_prompt_svg.md
```

- `SKILL.md`: main workflow and routing instructions.
- `agents/openai.yaml`: UI-facing skill metadata.
- `prompts/sam3_prompt_generator.md`: guidance for choosing `sam3_prompt`.
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

- [sam3 prompt generator](./prompts/sam3_prompt_generator.md): plan `sam3_prompt` and call `segment_and_extract_icons`.
- [draw.io template prompt](./prompts/template_prompt_drawio.md): create `{output_dir}/template.drawio`.
- [SVG template prompt](./prompts/template_prompt_svg.md): create `{output_dir}/template.svg`.

## Critical Resource Read Contract

When reading any prompt/resource file for this skill, read it from line 1 through EOF before using it.

If the file reader returns a bounded range such as lines 1-50 or lines 51-150, continue reading the next ranges until the explicit EOF marker is reached.

Do not proceed based on a partial read.

Required EOF markers:
- `prompts/sam3_prompt_generator.md` must end with `<!-- END_OF_FILE: sam3_prompt_generator.md -->`
- `prompts/template_prompt_drawio.md` must end with `<!-- END_OF_FILE: template_prompt_drawio.md -->`
- `prompts/template_prompt_svg.md` must end with `<!-- END_OF_FILE: template_prompt_svg.md -->`

If an EOF marker is not found, stop and report that the resource file was not fully read.


## Workflow

1. Determine the target format.
   - If the user explicitly asks for SVG, produce SVG.
   - If the user asks for both draw.io and SVG, produce both.
   - Otherwise default to draw.io.

2. Plan segmentation.
   - Read `prompts/sam3_prompt_generator.md`.
   - Inspect the original image.
   - Follow the prompt file's output requirements to choose the exact `sam3_prompt` string.

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
   - Reuse the second call's returned `output_dir`, `samed_image`, `boxlib_json`, and `icon_infos_json` for the rest of the workflow.
   - If the first result is acceptable, or if the misses are editable elements that should be redrawn, do not rerun segmentation.

5. Generate the editable template file in the MCP output directory.
   - For draw.io, read `prompts/template_prompt_drawio.md`.
     - Write directly to `{output_dir}/template.drawio`.
     - Set `template_path` to that relative path.
   - For SVG, read `prompts/template_prompt_svg.md`.
     - Write directly to `{output_dir}/template.svg`.
     - Set `template_path` to that relative path.
   - Use the original image, `samed_image`, and `boxlib_json` as inputs.
   - Preserve the original image coordinate system from `boxlib.json.image_size`.
   - Create every `<AF>xx` placeholder exactly as instructed by the selected template prompt.
   - Do not embed the original raster figure or cropped icons in the template.
   - Do not paste the full template code in chat unless the user explicitly asks; create the file in the workspace.

6. Call MCP tool `replace_template_placeholders`.
   - Required arguments:
     - `workspace_dir`: same absolute workspace path.
     - `template_path`: `{output_dir}/template.drawio` or `{output_dir}/template.svg`, relative to `workspace_dir`.
   - Leave `output_format` as `auto` unless the suffix is ambiguous.
   - Because the template is written beside `icon_infos.json`, `icon_infos_relative_path` is usually unnecessary.
   - The tool writes `final.drawio` or `final.svg` beside the template.

## Output Discipline

- Return the final relative path from the replacement tool.
- Mention the generated `template.*` and `final.*` paths.
- If segmentation finds no icons, still generate the editable template and run replacement.
- If producing both formats, reuse the same segmentation output and run template generation plus replacement once per format.

## Placeholder Rules

- Placeholder IDs must match `boxlib.json` labels after removing angle brackets: `<AF>01` becomes `AF01`.
- The replacement tool depends on those IDs and on `icon_infos.json`.
- Do not invent extra AF placeholders.
- Do not omit AF placeholders found in `boxlib.json`.
- Use draw.io as the default target because it is easier to edit in diagrams.net.
