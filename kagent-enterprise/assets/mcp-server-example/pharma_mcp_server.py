#!/usr/bin/env python3
"""
Pharmaceutical MCP Server
A demo server providing drug interaction checking and clinical trial information.
"""

import asyncio
from typing import Any
from mcp.server import Server
from mcp.types import Tool, TextContent, Resource, ResourceTemplate
import json

# Mock pharmaceutical database (in production, this would connect to real APIs)
DRUG_DATABASE = {
    "aspirin": {
        "name": "Aspirin",
        "class": "NSAID",
        "interactions": ["warfarin", "ibuprofen"],
        "uses": "Pain relief, anti-inflammatory, antiplatelet",
        "dosage": "325-650mg every 4-6 hours",
        "contraindications": ["bleeding disorders", "peptic ulcer"]
    },
    "warfarin": {
        "name": "Warfarin",
        "class": "Anticoagulant",
        "interactions": ["aspirin", "amoxicillin", "simvastatin"],
        "uses": "Blood clot prevention",
        "dosage": "2-10mg daily (individualized)",
        "contraindications": ["pregnancy", "active bleeding"]
    },
    "metformin": {
        "name": "Metformin",
        "class": "Antidiabetic",
        "interactions": ["alcohol", "contrast_dye"],
        "uses": "Type 2 diabetes management",
        "dosage": "500-2000mg daily with meals",
        "contraindications": ["kidney disease", "liver disease"]
    },
    "lisinopril": {
        "name": "Lisinopril",
        "class": "ACE Inhibitor",
        "interactions": ["potassium_supplements", "nsaids"],
        "uses": "Hypertension, heart failure",
        "dosage": "10-40mg daily",
        "contraindications": ["pregnancy", "angioedema history"]
    }
}

CLINICAL_TRIALS = {
    "diabetes": [
        {
            "id": "NCT05234567",
            "title": "Novel GLP-1 Agonist for Type 2 Diabetes",
            "phase": "Phase III",
            "status": "Recruiting",
            "location": "Multi-center US"
        },
        {
            "id": "NCT05234890",
            "title": "SGLT2 Inhibitor Cardiovascular Outcomes",
            "phase": "Phase III",
            "status": "Active",
            "location": "International"
        }
    ],
    "hypertension": [
        {
            "id": "NCT05235123",
            "title": "Combination Therapy for Resistant Hypertension",
            "phase": "Phase II",
            "status": "Recruiting",
            "location": "EU and US"
        }
    ],
    "oncology": [
        {
            "id": "NCT05236789",
            "title": "Immunotherapy for Advanced Melanoma",
            "phase": "Phase III",
            "status": "Recruiting",
            "location": "Multi-center International"
        }
    ]
}

app = Server("pharma-mcp-server")

@app.list_tools()
async def list_tools() -> list[Tool]:
    """List available pharmaceutical tools."""
    return [
        Tool(
            name="check_drug_interactions",
            description="Check for potential interactions between two or more medications. "
                       "Returns severity and clinical recommendations.",
            inputSchema={
                "type": "object",
                "properties": {
                    "drugs": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "List of drug names to check for interactions"
                    }
                },
                "required": ["drugs"]
            }
        ),
        Tool(
            name="get_medication_info",
            description="Retrieve detailed information about a specific medication including "
                       "drug class, uses, dosage, and contraindications.",
            inputSchema={
                "type": "object",
                "properties": {
                    "drug_name": {
                        "type": "string",
                        "description": "Name of the medication"
                    }
                },
                "required": ["drug_name"]
            }
        ),
        Tool(
            name="search_clinical_trials",
            description="Search for active clinical trials by medical condition or disease area.",
            inputSchema={
                "type": "object",
                "properties": {
                    "condition": {
                        "type": "string",
                        "description": "Medical condition or disease (e.g., diabetes, hypertension)"
                    }
                },
                "required": ["condition"]
            }
        )
    ]

@app.call_tool()
async def call_tool(name: str, arguments: Any) -> list[TextContent]:
    """Handle tool execution."""

    if name == "check_drug_interactions":
        drugs = [d.lower() for d in arguments["drugs"]]

        # Check if drugs exist in database
        unknown_drugs = [d for d in drugs if d not in DRUG_DATABASE]
        if unknown_drugs:
            return [TextContent(
                type="text",
                text=f" Unknown medications: {', '.join(unknown_drugs)}\n"
                     f"Available drugs: {', '.join(DRUG_DATABASE.keys())}"
            )]

        # Find interactions
        interactions = []
        for i, drug1 in enumerate(drugs):
            for drug2 in drugs[i+1:]:
                if drug2 in DRUG_DATABASE[drug1]["interactions"]:
                    interactions.append({
                        "drug1": DRUG_DATABASE[drug1]["name"],
                        "drug2": DRUG_DATABASE[drug2]["name"],
                        "severity": "Moderate to High",
                        "mechanism": f"{DRUG_DATABASE[drug1]['class']} interaction with {DRUG_DATABASE[drug2]['class']}",
                        "recommendation": "Consult prescriber. May require dose adjustment or monitoring."
                    })

        if not interactions:
            result = f" No known major interactions found between: {', '.join([DRUG_DATABASE[d]['name'] for d in drugs])}"
        else:
            result = f" **Drug Interaction Alert**\n\n"
            for idx, interaction in enumerate(interactions, 1):
                result += f"**Interaction {idx}:**\n"
                result += f"- **Drugs**: {interaction['drug1']}  {interaction['drug2']}\n"
                result += f"- **Severity**: {interaction['severity']}\n"
                result += f"- **Mechanism**: {interaction['mechanism']}\n"
                result += f"- **Recommendation**: {interaction['recommendation']}\n\n"

        return [TextContent(type="text", text=result)]

    elif name == "get_medication_info":
        drug_name = arguments["drug_name"].lower()

        if drug_name not in DRUG_DATABASE:
            return [TextContent(
                type="text",
                text=f"L Medication '{drug_name}' not found in database.\n"
                     f"Available medications: {', '.join(DRUG_DATABASE.keys())}"
            )]

        drug = DRUG_DATABASE[drug_name]
        result = f"""
= **Medication Information: {drug['name']}**

**Drug Class**: {drug['class']}

**Indications**: {drug['uses']}

**Typical Dosage**: {drug['dosage']}

**Known Interactions**: {', '.join([DRUG_DATABASE.get(i, {}).get('name', i) for i in drug['interactions']])}

**Contraindications**: {', '.join(drug['contraindications'])}

 *This is for informational purposes only. Always consult with a healthcare provider.*
"""
        return [TextContent(type="text", text=result)]

    elif name == "search_clinical_trials":
        condition = arguments["condition"].lower()

        trials = CLINICAL_TRIALS.get(condition, [])

        if not trials:
            return [TextContent(
                type="text",
                text=f"=, No clinical trials found for '{condition}'.\n"
                     f"Available conditions: {', '.join(CLINICAL_TRIALS.keys())}"
            )]

        result = f"=, **Clinical Trials for {condition.title()}**\n\n"
        for trial in trials:
            result += f"**{trial['title']}**\n"
            result += f"- **Trial ID**: {trial['id']}\n"
            result += f"- **Phase**: {trial['phase']}\n"
            result += f"- **Status**: {trial['status']}\n"
            result += f"- **Location**: {trial['location']}\n\n"

        return [TextContent(type="text", text=result)]

    else:
        return [TextContent(type="text", text=f"Unknown tool: {name}")]

@app.list_resources()
async def list_resources() -> list[Resource]:
    """List available pharmaceutical resources."""
    return [
        Resource(
            uri="pharma://formulary",
            name="Hospital Formulary",
            mimeType="application/json",
            description="Complete list of approved medications"
        )
    ]

async def main():
    """Run the MCP server."""
    from mcp.server.stdio import stdio_server

    async with stdio_server() as (read_stream, write_stream):
        await app.run(
            read_stream,
            write_stream,
            app.create_initialization_options()
        )

if __name__ == "__main__":
    asyncio.run(main())
