"""Convert a Biopython SeqRecord to JSON in the shape of insdc.Record.

Values are converted as Biopython's GenBank and EMBL writers read them, e.g.
annotations that Biopython passes through str() are stored as strings.
"""
from Bio.SeqFeature import (AfterPosition, BeforePosition, BetweenPosition, CompoundLocation, ExactPosition,
                            OneOfPosition, Reference, UnknownPosition, WithinPosition)


def position_to_json(pos):
    if isinstance(pos, BeforePosition):
        return {'kind': 'before', 'value': int(pos)}
    if isinstance(pos, AfterPosition):
        return {'kind': 'after', 'value': int(pos)}
    if isinstance(pos, WithinPosition):
        return {'kind': 'within', 'value': int(pos), 'left': pos._left, 'right': pos._right}
    if isinstance(pos, BetweenPosition):
        return {'kind': 'between', 'value': int(pos), 'left': pos._left, 'right': pos._right}
    if isinstance(pos, OneOfPosition):
        return {'kind': 'oneof', 'value': int(pos), 'choices': [position_to_json(p) for p in pos.position_choices]}
    if isinstance(pos, UnknownPosition):
        return {'kind': 'unknown'}
    if isinstance(pos, ExactPosition):
        return {'kind': 'exact', 'value': int(pos)}
    raise ValueError(f'unsupported position {pos!r}')


def location_to_json(location):
    if isinstance(location, CompoundLocation):
        return {'operator': location.operator, 'parts': [location_to_json(p) for p in location.parts]}
    return {'start': position_to_json(location.start), 'end': position_to_json(location.end),
            'strand': location.strand or 0, 'ref': location.ref or ''}


def value_to_json(value):
    if value is None:
        return {'kind': 'flag'}
    if isinstance(value, (bool, int)):
        return {'kind': 'int', 'text': str(value)}
    return {'kind': 'text', 'text': f'{value}'}


def qualifiers_to_json(qualifiers):
    out = []
    for key, values in qualifiers.items():
        if not isinstance(values, (list, tuple)):
            values = [values]
        out.append({'key': key, 'values': [value_to_json(v) for v in values]})
    return out


def as_list(value):
    if isinstance(value, (list, tuple)):
        return [str(v) for v in value]
    return [str(value)]


def record_to_json(record):
    a = record.annotations
    annotations = {'molecule_type': a.get('molecule_type', '')}
    for key in ('topology', 'data_file_division', 'date', 'gi', 'db_source'):
        if key in a:
            annotations[key] = str(a[key][0] if isinstance(a[key], list) else a[key])
    if 'segment' in a:
        annotations['segment'] = str(a['segment'][0] if isinstance(a['segment'], list) else a['segment'])
    for key in ('source', 'organism'):
        if key in a:
            annotations[key] = str(a[key][0] if isinstance(a[key], list) else a[key])
    for key in ('accessions', 'keywords', 'taxonomy'):
        if key in a:
            annotations[key] = [str(v) for v in a[key]]
    if 'comment' in a:
        comment = a['comment']
        annotations['comment'] = comment.split('\n') if isinstance(comment, str) else [str(c) for c in comment]
    if 'structured_comment' in a:
        annotations['structured_comment'] = [
            {'name': name, 'fields': [[k, str(v)] for k, v in fields.items()]}
            for name, fields in a['structured_comment'].items()]
    if 'references' in a:
        refs = []
        for ref in a['references']:
            if not isinstance(ref, Reference):
                continue
            entry = {k: getattr(ref, k) or '' for k in ('authors', 'consrtm', 'title', 'journal', 'medline_id', 'pubmed_id', 'comment')}
            if ref.location and len(ref.location) == 1:
                entry['location'] = [int(ref.location[0].start), int(ref.location[0].end)]
            refs.append(entry)
        annotations['references'] = refs
    return {
        'id': record.id,
        'name': record.name,
        'description': record.description,
        'seq': str(record.seq),
        'dbxrefs': list(record.dbxrefs),
        'annotations': annotations,
        'features': [{'type': f.type, 'location': location_to_json(f.location), 'qualifiers': qualifiers_to_json(f.qualifiers)}
                     for f in record.features],
    }
