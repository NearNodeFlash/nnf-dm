# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: datamovement.proto
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()


from google.protobuf import empty_pb2 as google_dot_protobuf_dot_empty__pb2


DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x12\x64\x61tamovement.proto\x12\x0c\x64\x61tamovement\x1a\x1bgoogle/protobuf/empty.proto\"C\n\x1b\x44\x61taMovementVersionResponse\x12\x0f\n\x07version\x18\x01 \x01(\t\x12\x13\n\x0b\x61piVersions\x18\x02 \x03(\t\"7\n\x14\x44\x61taMovementWorkflow\x12\x0c\n\x04name\x18\x01 \x01(\t\x12\x11\n\tnamespace\x18\x02 \x01(\t\"\x81\x01\n\x19\x44\x61taMovementCommandStatus\x12\x0f\n\x07\x63ommand\x18\x01 \x01(\t\x12\x10\n\x08progress\x18\x02 \x01(\x05\x12\x13\n\x0b\x65lapsedTime\x18\x03 \x01(\t\x12\x13\n\x0blastMessage\x18\x04 \x01(\t\x12\x17\n\x0flastMessageTime\x18\x05 \x01(\t\"\x8b\x02\n\x19\x44\x61taMovementCreateRequest\x12\x34\n\x08workflow\x18\x01 \x01(\x0b\x32\".datamovement.DataMovementWorkflow\x12\x0e\n\x06source\x18\x02 \x01(\t\x12\x13\n\x0b\x64\x65stination\x18\x03 \x01(\t\x12\x0e\n\x06\x64ryrun\x18\x04 \x01(\x08\x12\x15\n\rmpirunOptions\x18\x05 \x01(\t\x12\x12\n\ndcpOptions\x18\x06 \x01(\t\x12\x11\n\tlogStdout\x18\x07 \x01(\x08\x12\x13\n\x0bstoreStdout\x18\x08 \x01(\x08\x12\r\n\x05slots\x18\t \x01(\x05\x12\x10\n\x08maxSlots\x18\n \x01(\x05\x12\x0f\n\x07profile\x18\x0b \x01(\t\"\xab\x01\n\x1a\x44\x61taMovementCreateResponse\x12\x0b\n\x03uid\x18\x01 \x01(\t\x12?\n\x06status\x18\x02 \x01(\x0e\x32/.datamovement.DataMovementCreateResponse.Status\x12\x0f\n\x07message\x18\x03 \x01(\t\".\n\x06Status\x12\x0b\n\x07SUCCESS\x10\x00\x12\n\n\x06\x46\x41ILED\x10\x01\x12\x0b\n\x07INVALID\x10\x02\"s\n\x19\x44\x61taMovementStatusRequest\x12\x34\n\x08workflow\x18\x01 \x01(\x0b\x32\".datamovement.DataMovementWorkflow\x12\x0b\n\x03uid\x18\x02 \x01(\t\x12\x13\n\x0bmaxWaitTime\x18\x03 \x01(\x03\"\xd6\x03\n\x1a\x44\x61taMovementStatusResponse\x12=\n\x05state\x18\x01 \x01(\x0e\x32..datamovement.DataMovementStatusResponse.State\x12?\n\x06status\x18\x02 \x01(\x0e\x32/.datamovement.DataMovementStatusResponse.Status\x12\x0f\n\x07message\x18\x03 \x01(\t\x12>\n\rcommandStatus\x18\x04 \x01(\x0b\x32\'.datamovement.DataMovementCommandStatus\x12\x11\n\tstartTime\x18\x05 \x01(\t\x12\x0f\n\x07\x65ndTime\x18\x06 \x01(\t\"a\n\x05State\x12\x0b\n\x07PENDING\x10\x00\x12\x0c\n\x08STARTING\x10\x01\x12\x0b\n\x07RUNNING\x10\x02\x12\r\n\tCOMPLETED\x10\x03\x12\x0e\n\nCANCELLING\x10\x04\x12\x11\n\rUNKNOWN_STATE\x10\x05\"`\n\x06Status\x12\x0b\n\x07INVALID\x10\x00\x12\r\n\tNOT_FOUND\x10\x01\x12\x0b\n\x07SUCCESS\x10\x02\x12\n\n\x06\x46\x41ILED\x10\x03\x12\r\n\tCANCELLED\x10\x04\x12\x12\n\x0eUNKNOWN_STATUS\x10\x05\"^\n\x19\x44\x61taMovementDeleteRequest\x12\x34\n\x08workflow\x18\x01 \x01(\x0b\x32\".datamovement.DataMovementWorkflow\x12\x0b\n\x03uid\x18\x02 \x01(\t\"\xba\x01\n\x1a\x44\x61taMovementDeleteResponse\x12?\n\x06status\x18\x01 \x01(\x0e\x32/.datamovement.DataMovementDeleteResponse.Status\x12\x0f\n\x07message\x18\x02 \x01(\t\"J\n\x06Status\x12\x0b\n\x07INVALID\x10\x00\x12\r\n\tNOT_FOUND\x10\x01\x12\x0b\n\x07SUCCESS\x10\x02\x12\n\n\x06\x41\x43TIVE\x10\x03\x12\x0b\n\x07UNKNOWN\x10\x04\"O\n\x17\x44\x61taMovementListRequest\x12\x34\n\x08workflow\x18\x01 \x01(\x0b\x32\".datamovement.DataMovementWorkflow\"(\n\x18\x44\x61taMovementListResponse\x12\x0c\n\x04uids\x18\x01 \x03(\t\"^\n\x19\x44\x61taMovementCancelRequest\x12\x34\n\x08workflow\x18\x01 \x01(\x0b\x32\".datamovement.DataMovementWorkflow\x12\x0b\n\x03uid\x18\x02 \x01(\t\"\xad\x01\n\x1a\x44\x61taMovementCancelResponse\x12?\n\x06status\x18\x01 \x01(\x0e\x32/.datamovement.DataMovementCancelResponse.Status\x12\x0f\n\x07message\x18\x02 \x01(\t\"=\n\x06Status\x12\x0b\n\x07INVALID\x10\x00\x12\r\n\tNOT_FOUND\x10\x01\x12\x0b\n\x07SUCCESS\x10\x02\x12\n\n\x06\x46\x41ILED\x10\x03\x32\xb0\x04\n\tDataMover\x12N\n\x07Version\x12\x16.google.protobuf.Empty\x1a).datamovement.DataMovementVersionResponse\"\x00\x12]\n\x06\x43reate\x12\'.datamovement.DataMovementCreateRequest\x1a(.datamovement.DataMovementCreateResponse\"\x00\x12]\n\x06Status\x12\'.datamovement.DataMovementStatusRequest\x1a(.datamovement.DataMovementStatusResponse\"\x00\x12]\n\x06\x44\x65lete\x12\'.datamovement.DataMovementDeleteRequest\x1a(.datamovement.DataMovementDeleteResponse\"\x00\x12W\n\x04List\x12%.datamovement.DataMovementListRequest\x1a&.datamovement.DataMovementListResponse\"\x00\x12]\n\x06\x43\x61ncel\x12\'.datamovement.DataMovementCancelRequest\x1a(.datamovement.DataMovementCancelResponse\"\x00\x42W\n\x1d\x63om.hpe.cray.nnf.datamovementB\x11\x44\x61taMovementProtoP\x01Z!nnf.cray.hpe.com/datamovement/apib\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'datamovement_pb2', _globals)
if _descriptor._USE_C_DESCRIPTORS == False:
  DESCRIPTOR._options = None
  DESCRIPTOR._serialized_options = b'\n\035com.hpe.cray.nnf.datamovementB\021DataMovementProtoP\001Z!nnf.cray.hpe.com/datamovement/api'
  _globals['_DATAMOVEMENTVERSIONRESPONSE']._serialized_start=65
  _globals['_DATAMOVEMENTVERSIONRESPONSE']._serialized_end=132
  _globals['_DATAMOVEMENTWORKFLOW']._serialized_start=134
  _globals['_DATAMOVEMENTWORKFLOW']._serialized_end=189
  _globals['_DATAMOVEMENTCOMMANDSTATUS']._serialized_start=192
  _globals['_DATAMOVEMENTCOMMANDSTATUS']._serialized_end=321
  _globals['_DATAMOVEMENTCREATEREQUEST']._serialized_start=324
  _globals['_DATAMOVEMENTCREATEREQUEST']._serialized_end=591
  _globals['_DATAMOVEMENTCREATERESPONSE']._serialized_start=594
  _globals['_DATAMOVEMENTCREATERESPONSE']._serialized_end=765
  _globals['_DATAMOVEMENTCREATERESPONSE_STATUS']._serialized_start=719
  _globals['_DATAMOVEMENTCREATERESPONSE_STATUS']._serialized_end=765
  _globals['_DATAMOVEMENTSTATUSREQUEST']._serialized_start=767
  _globals['_DATAMOVEMENTSTATUSREQUEST']._serialized_end=882
  _globals['_DATAMOVEMENTSTATUSRESPONSE']._serialized_start=885
  _globals['_DATAMOVEMENTSTATUSRESPONSE']._serialized_end=1355
  _globals['_DATAMOVEMENTSTATUSRESPONSE_STATE']._serialized_start=1160
  _globals['_DATAMOVEMENTSTATUSRESPONSE_STATE']._serialized_end=1257
  _globals['_DATAMOVEMENTSTATUSRESPONSE_STATUS']._serialized_start=1259
  _globals['_DATAMOVEMENTSTATUSRESPONSE_STATUS']._serialized_end=1355
  _globals['_DATAMOVEMENTDELETEREQUEST']._serialized_start=1357
  _globals['_DATAMOVEMENTDELETEREQUEST']._serialized_end=1451
  _globals['_DATAMOVEMENTDELETERESPONSE']._serialized_start=1454
  _globals['_DATAMOVEMENTDELETERESPONSE']._serialized_end=1640
  _globals['_DATAMOVEMENTDELETERESPONSE_STATUS']._serialized_start=1566
  _globals['_DATAMOVEMENTDELETERESPONSE_STATUS']._serialized_end=1640
  _globals['_DATAMOVEMENTLISTREQUEST']._serialized_start=1642
  _globals['_DATAMOVEMENTLISTREQUEST']._serialized_end=1721
  _globals['_DATAMOVEMENTLISTRESPONSE']._serialized_start=1723
  _globals['_DATAMOVEMENTLISTRESPONSE']._serialized_end=1763
  _globals['_DATAMOVEMENTCANCELREQUEST']._serialized_start=1765
  _globals['_DATAMOVEMENTCANCELREQUEST']._serialized_end=1859
  _globals['_DATAMOVEMENTCANCELRESPONSE']._serialized_start=1862
  _globals['_DATAMOVEMENTCANCELRESPONSE']._serialized_end=2035
  _globals['_DATAMOVEMENTCANCELRESPONSE_STATUS']._serialized_start=1259
  _globals['_DATAMOVEMENTCANCELRESPONSE_STATUS']._serialized_end=1320
  _globals['_DATAMOVER']._serialized_start=2038
  _globals['_DATAMOVER']._serialized_end=2598
# @@protoc_insertion_point(module_scope)
