# Makefile for test-debug-labels

CXX = clang++
CXXFLAGS = -std=c++17 -O2 -Wall
MLX_PATH = $(HOME)/ml-explore/mlx
INCLUDES = -I$(MLX_PATH)
LDFLAGS = -L$(MLX_PATH)/build-release -lmlx -Wl,-rpath,$(MLX_PATH)/build-release -framework Metal -framework Foundation -framework Accelerate

TARGET_CPP = test-debug-labels
TARGET_OBJC = test-debug-labels-objc
SOURCE_CPP = main.cpp
SOURCE_OBJC = main_objc.m

all: $(TARGET_CPP) $(TARGET_OBJC)

$(TARGET_CPP): $(SOURCE_CPP)
	$(CXX) $(CXXFLAGS) $(INCLUDES) -o $(TARGET_CPP) $(SOURCE_CPP) $(LDFLAGS)

$(TARGET_OBJC): $(SOURCE_OBJC)
	clang -o $(TARGET_OBJC) $(SOURCE_OBJC) -framework Metal -framework Foundation

run-cpp: $(TARGET_CPP)
	MTL_CAPTURE_ENABLED=1 ./$(TARGET_CPP)

run-objc: $(TARGET_OBJC)
	./$(TARGET_OBJC)

run: all
	@echo "Run 'make run-cpp' or 'make run-objc'"

clean:
	rm -f $(TARGET_CPP) $(TARGET_OBJC) *.gputrace

.PHONY: all run clean
