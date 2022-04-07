# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# source: rsyncdatamovement.proto
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import message as _message
from google.protobuf import reflection as _reflection
from google.protobuf import symbol_database as _symbol_database
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x17rsyncdatamovement.proto\x12\x0c\x64\x61tamovement\"\x9d\x01\n\x1eRsyncDataMovementCreateRequest\x12\x11\n\tinitiator\x18\x01 \x01(\t\x12\x0e\n\x06target\x18\x02 \x01(\t\x12\x10\n\x08workflow\x18\x03 \x01(\t\x12\x11\n\tnamespace\x18\x04 \x01(\t\x12\x0e\n\x06source\x18\x05 \x01(\t\x12\x13\n\x0b\x64\x65stination\x18\x06 \x01(\t\x12\x0e\n\x06\x64ryrun\x18\x07 \x01(\x08\".\n\x1fRsyncDataMovementCreateResponse\x12\x0b\n\x03uid\x18\x01 \x01(\t\"-\n\x1eRsyncDataMovementStatusRequest\x12\x0b\n\x03uid\x18\x01 \x01(\t\"\xe2\x02\n\x1fRsyncDataMovementStatusResponse\x12\x42\n\x05state\x18\x01 \x01(\x0e\x32\x33.datamovement.RsyncDataMovementStatusResponse.State\x12\x44\n\x06status\x18\x02 \x01(\x0e\x32\x34.datamovement.RsyncDataMovementStatusResponse.Status\x12\x0f\n\x07message\x18\x03 \x01(\t\"Q\n\x05State\x12\x0b\n\x07PENDING\x10\x00\x12\x0c\n\x08STARTING\x10\x01\x12\x0b\n\x07RUNNING\x10\x02\x12\r\n\tCOMPLETED\x10\x03\x12\x11\n\rUNKNOWN_STATE\x10\x04\"Q\n\x06Status\x12\x0b\n\x07INVALID\x10\x00\x12\r\n\tNOT_FOUND\x10\x01\x12\x0b\n\x07SUCCESS\x10\x02\x12\n\n\x06\x46\x41ILED\x10\x03\x12\x12\n\x0eUNKNOWN_STATUS\x10\x04\"-\n\x1eRsyncDataMovementDeleteRequest\x12\x0b\n\x03uid\x18\x01 \x01(\t\"\xb3\x01\n\x1fRsyncDataMovementDeleteResponse\x12\x44\n\x06status\x18\x01 \x01(\x0e\x32\x34.datamovement.RsyncDataMovementDeleteResponse.Status\"J\n\x06Status\x12\x0b\n\x07INVALID\x10\x00\x12\r\n\tNOT_FOUND\x10\x01\x12\x0b\n\x07\x44\x45LETED\x10\x02\x12\n\n\x06\x41\x43TIVE\x10\x03\x12\x0b\n\x07UNKNOWN\x10\x04\x32\xcb\x02\n\x0eRsyncDataMover\x12g\n\x06\x43reate\x12,.datamovement.RsyncDataMovementCreateRequest\x1a-.datamovement.RsyncDataMovementCreateResponse\"\x00\x12g\n\x06Status\x12,.datamovement.RsyncDataMovementStatusRequest\x1a-.datamovement.RsyncDataMovementStatusResponse\"\x00\x12g\n\x06\x44\x65lete\x12,.datamovement.RsyncDataMovementDeleteRequest\x1a-.datamovement.RsyncDataMovementDeleteResponse\"\x00\x42W\n\x1d\x63om.hpe.cray.nnf.datamovementB\x11\x44\x61taMovementProtoP\x01Z!nnf.cray.hpe.com/datamovement/apib\x06proto3')



_RSYNCDATAMOVEMENTCREATEREQUEST = DESCRIPTOR.message_types_by_name['RsyncDataMovementCreateRequest']
_RSYNCDATAMOVEMENTCREATERESPONSE = DESCRIPTOR.message_types_by_name['RsyncDataMovementCreateResponse']
_RSYNCDATAMOVEMENTSTATUSREQUEST = DESCRIPTOR.message_types_by_name['RsyncDataMovementStatusRequest']
_RSYNCDATAMOVEMENTSTATUSRESPONSE = DESCRIPTOR.message_types_by_name['RsyncDataMovementStatusResponse']
_RSYNCDATAMOVEMENTDELETEREQUEST = DESCRIPTOR.message_types_by_name['RsyncDataMovementDeleteRequest']
_RSYNCDATAMOVEMENTDELETERESPONSE = DESCRIPTOR.message_types_by_name['RsyncDataMovementDeleteResponse']
_RSYNCDATAMOVEMENTSTATUSRESPONSE_STATE = _RSYNCDATAMOVEMENTSTATUSRESPONSE.enum_types_by_name['State']
_RSYNCDATAMOVEMENTSTATUSRESPONSE_STATUS = _RSYNCDATAMOVEMENTSTATUSRESPONSE.enum_types_by_name['Status']
_RSYNCDATAMOVEMENTDELETERESPONSE_STATUS = _RSYNCDATAMOVEMENTDELETERESPONSE.enum_types_by_name['Status']
RsyncDataMovementCreateRequest = _reflection.GeneratedProtocolMessageType('RsyncDataMovementCreateRequest', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTCREATEREQUEST,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementCreateRequest)
  })
_sym_db.RegisterMessage(RsyncDataMovementCreateRequest)

RsyncDataMovementCreateResponse = _reflection.GeneratedProtocolMessageType('RsyncDataMovementCreateResponse', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTCREATERESPONSE,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementCreateResponse)
  })
_sym_db.RegisterMessage(RsyncDataMovementCreateResponse)

RsyncDataMovementStatusRequest = _reflection.GeneratedProtocolMessageType('RsyncDataMovementStatusRequest', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTSTATUSREQUEST,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementStatusRequest)
  })
_sym_db.RegisterMessage(RsyncDataMovementStatusRequest)

RsyncDataMovementStatusResponse = _reflection.GeneratedProtocolMessageType('RsyncDataMovementStatusResponse', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTSTATUSRESPONSE,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementStatusResponse)
  })
_sym_db.RegisterMessage(RsyncDataMovementStatusResponse)

RsyncDataMovementDeleteRequest = _reflection.GeneratedProtocolMessageType('RsyncDataMovementDeleteRequest', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTDELETEREQUEST,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementDeleteRequest)
  })
_sym_db.RegisterMessage(RsyncDataMovementDeleteRequest)

RsyncDataMovementDeleteResponse = _reflection.GeneratedProtocolMessageType('RsyncDataMovementDeleteResponse', (_message.Message,), {
  'DESCRIPTOR' : _RSYNCDATAMOVEMENTDELETERESPONSE,
  '__module__' : 'rsyncdatamovement_pb2'
  # @@protoc_insertion_point(class_scope:datamovement.RsyncDataMovementDeleteResponse)
  })
_sym_db.RegisterMessage(RsyncDataMovementDeleteResponse)

_RSYNCDATAMOVER = DESCRIPTOR.services_by_name['RsyncDataMover']
if _descriptor._USE_C_DESCRIPTORS == False:

  DESCRIPTOR._options = None
  DESCRIPTOR._serialized_options = b'\n\035com.hpe.cray.nnf.datamovementB\021DataMovementProtoP\001Z!nnf.cray.hpe.com/datamovement/api'
  _RSYNCDATAMOVEMENTCREATEREQUEST._serialized_start=42
  _RSYNCDATAMOVEMENTCREATEREQUEST._serialized_end=199
  _RSYNCDATAMOVEMENTCREATERESPONSE._serialized_start=201
  _RSYNCDATAMOVEMENTCREATERESPONSE._serialized_end=247
  _RSYNCDATAMOVEMENTSTATUSREQUEST._serialized_start=249
  _RSYNCDATAMOVEMENTSTATUSREQUEST._serialized_end=294
  _RSYNCDATAMOVEMENTSTATUSRESPONSE._serialized_start=297
  _RSYNCDATAMOVEMENTSTATUSRESPONSE._serialized_end=651
  _RSYNCDATAMOVEMENTSTATUSRESPONSE_STATE._serialized_start=487
  _RSYNCDATAMOVEMENTSTATUSRESPONSE_STATE._serialized_end=568
  _RSYNCDATAMOVEMENTSTATUSRESPONSE_STATUS._serialized_start=570
  _RSYNCDATAMOVEMENTSTATUSRESPONSE_STATUS._serialized_end=651
  _RSYNCDATAMOVEMENTDELETEREQUEST._serialized_start=653
  _RSYNCDATAMOVEMENTDELETEREQUEST._serialized_end=698
  _RSYNCDATAMOVEMENTDELETERESPONSE._serialized_start=701
  _RSYNCDATAMOVEMENTDELETERESPONSE._serialized_end=880
  _RSYNCDATAMOVEMENTDELETERESPONSE_STATUS._serialized_start=806
  _RSYNCDATAMOVEMENTDELETERESPONSE_STATUS._serialized_end=880
  _RSYNCDATAMOVER._serialized_start=883
  _RSYNCDATAMOVER._serialized_end=1214
# @@protoc_insertion_point(module_scope)
