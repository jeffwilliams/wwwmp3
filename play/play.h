#ifndef PLAY_H
#define PLAY_H

#include <mpg123.h>
#include <ao/ao.h>

typedef struct {
  char* title;
  char* artist;
  char* album;
} play_metadata_t;

typedef struct {
  mpg123_handle* mh;
  size_t buffer_size;
  unsigned char *buffer;
} play_reader_t;

void play_init();
void play_free();
int play_setvolume(unsigned char pct);
void play_play(char* filename);

play_reader_t* play_new_reader(char* filename);
void play_delete_reader(play_reader_t* reader);
size_t play_read(play_reader_t* reader);
int play_length(play_reader_t* reader);
int play_offset(play_reader_t* reader);
void play_seek(play_reader_t* reader, int offset);

ao_device* play_new_writer(play_reader_t* reader);
void play_delete_writer(ao_device* writer);
void play_write(ao_device* writer, unsigned char* buffer, size_t done);

#ifdef __cplusplus
extern "C" {
#endif
  play_metadata_t play_meta(char* filename);
  void play_delete_meta(play_metadata_t meta);
  void play_debug_meta(char* filename);
#ifdef __cplusplus
}
#endif

#endif //PLAY_H
