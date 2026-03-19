#!/usr/bin/env python3
"""
Generate JetBrains Qodana Dockerfiles

Usage:
    python dockerfiles.py 2026.1
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

def load_release_info(qd_code: str, qd_version: str) -> Dict[str, Any]:
    """
    Load release information from local feed files for the specified product and version.

    Args:
        qd_code (str): The Qodana product code (e.g., "QDGO", "QDJVM").
        qd_version (str): The Qodana major version (e.g., "2026.1").

    Returns:
        Dict[str, Any]: A dictionary containing "build" and "downloads" keys,
                        or an empty dict if the release cannot be found.
    """
    # Map QD_CODE to feed file name
    # Note: QDAND and QDANDC use JVM and JVM-Community feeds respectively
    product_mapping = {
        "QDGO": "go",
        "QDJS": "js",
        "QDJVM": "jvm",
        "QDJVMC": "jvm-community",
        "QDAND": "jvm",           # Android uses JVM feed
        "QDANDC": "jvm-community", # Android Community uses JVM-Community feed
        "QDNET": "dotnet",
        "QDPHP": "php",
        "QDPY": "python",
        "QDPYC": "python-community",
        "QDCPP": "cpp",
        "QDRUBY": "ruby",
    }

    product_name = product_mapping.get(qd_code)
    if not product_name:
        logger.warning("Unknown product code '%s'. Skipping release info lookup.", qd_code)
        return {}

    feed_path = os.path.join("feed", f"qodana-{product_name}.releases.json")

    if not os.path.isfile(feed_path):
        logger.warning("Feed file '%s' not found for product '%s'. Skipping.", feed_path, qd_code)
        return {}

    try:
        with open(feed_path, "r", encoding="utf-8") as f:
            feed_data = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        logger.warning("Error reading feed file '%s': %s. Skipping.", feed_path, e)
        return {}

    releases = feed_data.get("Releases", [])
    if not releases:
        logger.warning("No releases found in feed file '%s'. Skipping.", feed_path)
        return {}

    # Sort releases by Type and Date, then filter by MajorVersion
    sorted_releases = sorted(releases, key=lambda r: (r.get("Type", ""), r.get("Date", "")))
    matching_releases = [r for r in sorted_releases if r.get("MajorVersion") == qd_version]

    if not matching_releases:
        logger.warning(
            "No release found for %s version %s in %s. Skipping.",
            qd_code, qd_version, feed_path
        )
        return {}

    # Take the latest matching release
    latest_release = matching_releases[-1]

    return {
        "build": latest_release.get("Build", ""),
        "downloads": latest_release.get("Downloads", {})
    }

def parse_args() -> str:
    """
    Parse command-line arguments and return the Qodana version.

    Returns:
        str: The Qodana major version (e.g., "2026.1").
    """
    parser = argparse.ArgumentParser(description="Generate Dockerfiles from base templates.")
    parser.add_argument(
        "qd_version",
        help="Qodana major version (e.g., 2026.1)."
    )
    args = parser.parse_args()
    return args.qd_version

def load_variants() -> Dict[str, Any]:
    """
    Load variant definitions from dockerfiles/public.json.

    Returns:
        Dict[str, Any]: A dictionary of variants from the JSON file.

    Raises:
        SystemExit: If the file is missing, cannot be read, or is invalid JSON.
    """
    public_json_path = "dockerfiles/public.json"
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
    qd_version: str
) -> str:
    """
    Generate the final Dockerfile content for a specific variant.

    Args:
        variant (str): The name of the variant.
        data (Dict[str, Any]): Variant-specific metadata from the JSON file.
        base_dockerfile_dir (str): Path to the directory containing base Dockerfiles.
        intellij_template (Template): Jinja2 template for IntelliJ-based variants.
        thirdparty_template (Template): Jinja2 template for third-party variants.
        qd_version (str): The Qodana major version (e.g., "2026.1").

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

    is_third_party = data.get("is_third_party", False)
    qd_code = data.get("qd_code", "")

    if is_third_party:
        # Third-party variants don't need release info from feeds
        template = thirdparty_template
        snippet = template.render(
            qd_version=qd_version,
            qd_code=qd_code,
            description=data.get("description", ""),
            variant=variant.split("-")[0],
            qd_image=variant
        )
    else:
        # For IntelliJ-based variants (not third-party), load release info from local feeds
        release_info = load_release_info(qd_code, qd_version)
        if not release_info:
            # If we can't find release info, skip this variant
            logger.warning("Skipping variant '%s' due to missing release information.", variant)
            return ""

        template = intellij_template
        snippet = template.render(
            qd_version=qd_version,
            qd_code=qd_code,
            qd_build=release_info.get("build", ""),
            qd_downloads=release_info.get("downloads", {}),
            description=data.get("description", ""),
            variant=variant.split("-")[0],
            qd_image=variant
        )

    final_dockerfile = processed_base_content.rstrip() + "\n\n" + snippet
    return final_dockerfile

def write_dockerfile(variant: str, dockerfile_content: str) -> None:
    """
    Write the final Dockerfile content to the appropriate output directory.

    Args:
        variant (str): The variant name.
        dockerfile_content (str): The complete Dockerfile content to write.
    """
    if not dockerfile_content:
        logger.debug("No Dockerfile content to write for variant '%s'. Skipping.", variant)
        return
    generated_disclaimer = "# This file was generated by https://github.com/JetBrains/qodana-cli/blob/main/scripts/dockerfiles.py. DO NOT EDIT MANUALLY."
    dockerfile_content = f"{generated_disclaimer}\n\n{dockerfile_content}"

    out_dir = os.path.join("dockerfiles", variant)
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
    qd_version = parse_args()
    variants = load_variants()

    env = create_jinja_environment()

    intellij_template_path = "dockerfiles/base/templates/intellij.Dockerfile.j2"
    thirdparty_template_path = "dockerfiles/base/templates/thirdparty.Dockerfile.j2"
    intellij_template = load_template(env, intellij_template_path)
    thirdparty_template = load_template(env, thirdparty_template_path)

    base_dockerfile_dir = "dockerfiles/base"

    for variant, data in variants.items():
        dockerfile_content = generate_variant_dockerfile(
            variant,
            data,
            base_dockerfile_dir,
            intellij_template,
            thirdparty_template,
            qd_version
        )
        write_dockerfile(variant, dockerfile_content)

if __name__ == "__main__":
    main()