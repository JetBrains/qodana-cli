#!/usr/bin/env python3
"""
Generate JetBrains JetBrains/qodana-docker

Usage:
    python dockerfiles.py /path/to/release_dir
"""
import argparse
import json
import logging
import os
import re
import sys
from typing import Any, Dict

from jinja2 import Environment, FileSystemLoader, Template, select_autoescape

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def parse_args() -> str:
    """
    Parse command-line arguments and return the release directory path.

    Returns:
        str: The path to the release directory.
    """
    parser = argparse.ArgumentParser(description="Generate Dockerfiles from base templates.")
    parser.add_argument(
        "release_dir",
        help="Path to the release directory containing public.json and template files."
    )
    args = parser.parse_args()
    return args.release_dir

def validate_release_dir(release_dir: str) -> None:
    """
    Validate that the release directory exists and is a directory.

    Args:
        release_dir (str): The path to the release directory.

    Raises:
        SystemExit: If the directory does not exist.
    """
    if not os.path.isdir(release_dir):
        logger.error("Release directory '%s' doesn't exist.", release_dir)
        sys.exit(1)

def load_variants(release_dir: str) -> Dict[str, Any]:
    """
    Load variant definitions from public.json in the release directory.

    Args:
        release_dir (str): The path to the release directory.

    Returns:
        Dict[str, Any]: A dictionary of variants from the JSON file.

    Raises:
        SystemExit: If the file is missing, cannot be read, or is invalid JSON.
    """
    public_json_path = os.path.join(release_dir, "public.json")
    if not os.path.isfile(public_json_path):
        logger.error("'%s' not found.", public_json_path)
        sys.exit(1)

    try:
        with open(public_json_path, "r", encoding="utf-8") as file:
            variants = json.load(file)
        return variants
    except json.JSONDecodeError as e:
        logger.error("Error decoding JSON from '%s': %s", public_json_path, e)
        sys.exit(1)
    except OSError as e:
        logger.error("Error reading '%s': %s", public_json_path, e)
        sys.exit(1)

def create_jinja_environment() -> Environment:
    """
    Create and return a Jinja2 environment configured for file system loading and autoescaping.

    Returns:
        Environment: The configured Jinja2 environment.
    """
    return Environment(
        loader=FileSystemLoader("."),
        autoescape=select_autoescape()
    )

def load_template(env: Environment, template_path: str) -> Template:
    """
    Load a Jinja2 template from the given path using the provided environment.

    Args:
        env (Environment): The Jinja2 environment to use.
        template_path (str): The file path of the template.

    Returns:
        Template: The loaded Jinja2 template object.

    Raises:
        SystemExit: If the template cannot be loaded.
    """
    try:
        return env.get_template(template_path)
    except Exception as e:
        logger.error("Error loading template '%s': %s", template_path, e)
        sys.exit(1)

def substitute_from_directives(content: str, base_dir: str) -> str:
    """
    Recursively replace lines of the form:
        FROM identifier
    where 'identifier' is composed of letters and dashes, with the contents of
    <identifier>.Dockerfile found in 'base_dir'.

    Args:
        content (str): The Dockerfile content to process.
        base_dir (str): The directory containing base Dockerfiles to include.

    Returns:
        str: The processed Dockerfile content with all FROM directives substituted.
    """
    pattern = re.compile(r"^(FROM)\s+([A-Za-z-]+)\s*$")
    lines = content.splitlines()
    new_lines = []

    for line in lines:
        match = pattern.match(line)
        if match:
            identifier = match.group(2)
            file_path = os.path.join(base_dir, f"{identifier}.Dockerfile")
            if os.path.isfile(file_path):
                try:
                    with open(file_path, "r", encoding="utf-8") as inc_file:
                        included_content = inc_file.read()
                    # Recursively process the included file's content
                    substituted_content = substitute_from_directives(included_content, base_dir)
                    new_lines.append(substituted_content.rstrip())
                except OSError as e:
                    logger.error("Error reading included file '%s': %s", file_path, e)
                    new_lines.append(line)
            else:
                # If no file found, leave the line as is.
                new_lines.append(line)
        else:
            new_lines.append(line)

    return "\n".join(new_lines)

def generate_variant_dockerfile(
    variant: str,
    data: Dict[str, Any],
    base_dockerfile_dir: str,
    intellij_template: Template,
    thirdparty_template: Template,
    release_dir: str
) -> str:
    """
    Generate the final Dockerfile content for a specific variant.

    Args:
        variant (str): The name of the variant.
        data (Dict[str, Any]): Variant-specific metadata from the JSON file.
        base_dockerfile_dir (str): Path to the directory containing base Dockerfiles.
        intellij_template (Template): Jinja2 template for IntelliJ-based variants.
        thirdparty_template (Template): Jinja2 template for third-party variants.
        release_dir (str): The main release directory path.

    Returns:
        str: The final Dockerfile content, or an empty string if an error occurred.
    """
    base_source = data.get("from", variant)
    base_dockerfile_path = os.path.join(base_dockerfile_dir, f"{base_source}.Dockerfile")

    if not os.path.isfile(base_dockerfile_path):
        logger.warning("Skipping %s: %s not found.", variant, base_dockerfile_path)
        return ""

    # Read and process the base Dockerfile content with recursive substitutions
    try:
        with open(base_dockerfile_path, "r", encoding="utf-8") as f:
            base_content = f.read()
        processed_base_content = substitute_from_directives(base_content, base_dockerfile_dir)
    except OSError as e:
        logger.error("Error processing base Dockerfile for variant '%s': %s", variant, e)
        return ""

    template = thirdparty_template if data.get("is_third_party", False) else intellij_template
    snippet = template.render(
        qd_release=release_dir,
        qd_code=data.get("qd_code", ""),
        description=data.get("description", ""),
        variant=variant.split("-")[0],
        qd_image=variant
    )

    final_dockerfile = processed_base_content.rstrip() + "\n\n" + snippet
    return final_dockerfile

def write_dockerfile(variant: str, release_dir: str, dockerfile_content: str) -> None:
    """
    Write the final Dockerfile content to the appropriate output directory.

    Args:
        variant (str): The variant name.
        release_dir (str): The path to the release directory.
        dockerfile_content (str): The complete Dockerfile content to write.
    """
    if not dockerfile_content:
        logger.debug("No Dockerfile content to write for variant '%s'. Skipping.", variant)
        return
    generated_disclaimer = "# This file was generated by https://github.com/JetBrains/qodana-docker/blob/main/dockerfiles.py. DO NOT EDIT MANUALLY."
    dockerfile_content = f"{generated_disclaimer}\n\n{dockerfile_content}"

    out_dir = os.path.join(release_dir, variant)
    out_path = os.path.join(out_dir, "Dockerfile")

    os.makedirs(out_dir, exist_ok=True)
    try:
        with open(out_path, "w", encoding="utf-8") as out_file:
            out_file.write(dockerfile_content)
        logger.info("Generated %s.", out_path)
    except OSError as e:
        logger.error("Error writing output for variant '%s': %s", variant, e)

def main() -> None:
    """
    Main entry point: parse arguments, load variants, load templates, and generate Dockerfiles.
    """
    release_dir = parse_args()
    validate_release_dir(release_dir)
    variants = load_variants(release_dir)

    env = create_jinja_environment()

    intellij_template_path = os.path.join(release_dir, "base", "templates", "intellij.Dockerfile.j2")
    thirdparty_template_path = os.path.join(release_dir, "base", "templates", "thirdparty.Dockerfile.j2")
    intellij_template = load_template(env, intellij_template_path)
    thirdparty_template = load_template(env, thirdparty_template_path)

    base_dockerfile_dir = os.path.join(release_dir, "base")

    for variant, data in variants.items():
        dockerfile_content = generate_variant_dockerfile(
            variant,
            data,
            base_dockerfile_dir,
            intellij_template,
            thirdparty_template,
            release_dir
        )
        write_dockerfile(variant, release_dir, dockerfile_content)

if __name__ == "__main__":
    main()